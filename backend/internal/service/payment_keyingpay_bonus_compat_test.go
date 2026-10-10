//go:build unit

package service

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	"github.com/stretchr/testify/require"
)

// Exercise the real V2 signature verifier and fulfillment entry together: the
// gateway settles CNY, while the balance order credits USD including its bonus.
func TestKeyingPaySignedBonusAndDiscountCallbacksCreditOnce(t *testing.T) {
	merchantKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	platformKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	merchantDER, err := x509.MarshalPKCS8PrivateKey(merchantKey)
	require.NoError(t, err)
	platformDER, err := x509.MarshalPKIXPublicKey(&platformKey.PublicKey)
	require.NoError(t, err)
	keying, err := provider.NewKeyingPay("bonus-compat", map[string]string{
		"pid":                "1001",
		"merchantPrivateKey": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: merchantDER})),
		"platformPublicKey":  string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: platformDER})),
		"notifyUrl":          "https://merchant.example/api/v1/payment/webhook/keyingpay",
		"returnUrl":          "https://merchant.example/payment/result",
	})
	require.NoError(t, err)

	for _, tc := range []struct {
		mode             string
		wantCredit       float64
		wantGatewayMoney float64
	}{
		{mode: RechargeBonusModeBonus, wantCredit: 16.8, wantGatewayMoney: 103},
		{mode: RechargeBonusModeDiscount, wantCredit: 14, wantGatewayMoney: 82.4},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			quote := quoteRechargeBonus(&PaymentConfig{
				BalanceRechargeMultiplier: 0.14,
				RechargeBonusMode:         tc.mode,
				RechargeBonusTiers:        []RechargeBonusTier{{MinAmount: 100, BonusPercent: 20}},
			}, 100, "CNY")
			require.InDelta(t, tc.wantCredit, quote.Credited, 1e-8)
			require.InDelta(t, 2.8, quote.Bonus, 1e-8)
			_, payAmount, err := calculateCreateOrderPayAmount(quote.PayBase, 3, "CNY")
			require.NoError(t, err)
			require.InDelta(t, tc.wantGatewayMoney, payAmount, 1e-8)

			order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPending, time.Now())
			order, err = client.PaymentOrder.UpdateOneID(order.ID).
				SetOrderType(payment.OrderTypeBalance).
				ClearPlanID().ClearSubscriptionGroupID().ClearSubscriptionDays().
				ClearPaidAt().SetPaymentTradeNo("").
				SetAmount(quote.Credited).SetPayAmount(payAmount).SetBonusAmount(quote.Bonus).
				SetProviderKey(payment.TypeKeyingPay).
				SetProviderSnapshot(buildPaymentOrderProviderSnapshot(&payment.InstanceSelection{
					ProviderKey: payment.TypeKeyingPay,
					Config:      map[string]string{"pid": "1001"},
				}, CreateOrderRequest{PaymentType: payment.TypeAlipay})).Save(ctx)
			require.NoError(t, err)
			require.NoError(t, client.User.UpdateOneID(order.UserID).SetBalance(10).Exec(ctx))

			redeemRepo := &paymentFulfillmentRedeemRepo{}
			creditCalls := 0
			userRepo := &mockUserRepo{getByIDUser: &User{ID: order.UserID, Balance: 10}}
			userRepo.updateBalanceFn = func(txCtx context.Context, id int64, amount float64) error {
				require.Equal(t, order.UserID, id)
				require.InDelta(t, tc.wantCredit, amount, 1e-8)
				creditCalls++
				tx := dbent.TxFromContext(txCtx)
				require.NotNil(t, tx, "the credit must run within the redemption transaction")
				return tx.User.UpdateOneID(id).AddBalance(amount).Exec(txCtx)
			}
			svc := &PaymentService{entClient: client, userRepo: userRepo,
				redeemService: NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil)}

			// Even a genuinely signed notification must match the paid CNY amount,
			// never the different USD credit amount stored in Amount.
			wrong := signedKeyingPayBonusCallback(t, platformKey, order.OutTradeNo, quote.Credited)
			wrongNotification, err := keying.VerifyNotification(ctx, wrong.Encode(), nil)
			require.NoError(t, err)
			require.ErrorContains(t, svc.HandlePaymentNotification(ctx, wrongNotification, payment.TypeKeyingPay), "amount mismatch")
			reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusPending, reloaded.Status)
			require.Zero(t, creditCalls)

			callback := signedKeyingPayBonusCallback(t, platformKey, order.OutTradeNo, payAmount)
			tampered, err := url.ParseQuery(callback.Encode())
			require.NoError(t, err)
			tampered.Set("money", "999.00")
			_, err = keying.VerifyNotification(ctx, tampered.Encode(), nil)
			require.ErrorContains(t, err, "invalid signature")

			for range 3 {
				notification, err := keying.VerifyNotification(ctx, callback.Encode(), nil)
				require.NoError(t, err)
				require.NotNil(t, notification)
				require.NoError(t, svc.HandlePaymentNotification(ctx, notification, payment.TypeKeyingPay))
			}
			reloaded, err = client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusCompleted, reloaded.Status)
			require.InDelta(t, payAmount, reloaded.PayAmount, 1e-8)
			require.InDelta(t, tc.wantCredit, reloaded.Amount, 1e-8)
			require.InDelta(t, 2.8, reloaded.BonusAmount, 1e-8)
			user, err := client.User.Get(ctx, order.UserID)
			require.NoError(t, err)
			require.InDelta(t, 10+tc.wantCredit, user.Balance, 1e-8)
			require.Equal(t, 1, creditCalls)
			require.Equal(t, 1, redeemRepo.createCalls)
			require.Len(t, redeemRepo.useCalls, 1)
		})
	}
}

func signedKeyingPayBonusCallback(t *testing.T, platformKey *rsa.PrivateKey, orderID string, amount float64) url.Values {
	t.Helper()
	fields := url.Values{
		"pid": {"1001"}, "out_trade_no": {orderID}, "trade_no": {"KP-BONUS-COMPAT-" + orderID},
		"money": {payment.FormatAmountForCurrency(amount, "CNY")}, "type": {payment.TypeAlipay},
		"trade_status": {"TRADE_SUCCESS"}, "timestamp": {strconv.FormatInt(time.Now().Unix(), 10)},
		"sign_type": {"RSA"},
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		if key != "sign_type" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+fields.Get(key))
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "&")))
	signature, err := rsa.SignPKCS1v15(rand.Reader, platformKey, crypto.SHA256, digest[:])
	require.NoError(t, err)
	fields.Set("sign", base64.StdEncoding.EncodeToString(signature))
	return fields
}
