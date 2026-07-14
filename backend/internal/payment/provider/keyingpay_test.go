package provider

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

var (
	keyingPayTestKeysOnce sync.Once
	keyingPayMerchantKey  *rsa.PrivateKey
	keyingPayPlatformKey  *rsa.PrivateKey
	keyingPayTestKeysErr  error
)

func TestKeyingPayCanonicalSigning(t *testing.T) {
	merchantKey, _ := keyingPayTestKeys(t)
	values := url.Values{
		"b":         {"two"},
		"a":         {"one"},
		"empty":     {""},
		"sign_type": {keyingPaySignType},
	}

	fields, err := strictKeyingPayFormFields(values)
	require.NoError(t, err)
	require.Equal(t, "a=one&b=two", canonicalKeyingPayFields(fields))

	signature, err := signKeyingPayValues(values, merchantKey)
	require.NoError(t, err)
	require.NoError(t, verifyKeyingPaySignature("a=one&b=two", signature, &merchantKey.PublicKey))
	require.Error(t, verifyKeyingPaySignature("a=changed&b=two", signature, &merchantKey.PublicKey))
}

func TestNewKeyingPayAcceptsPEMAndBareBase64Keys(t *testing.T) {
	merchantKey, platformKey := keyingPayTestKeys(t)
	merchantDER, err := x509.MarshalPKCS8PrivateKey(merchantKey)
	require.NoError(t, err)
	platformDER, err := x509.MarshalPKIXPublicKey(&platformKey.PublicKey)
	require.NoError(t, err)

	provider, err := NewKeyingPay("instance-1", map[string]string{
		"pid":                "1001",
		"merchantPrivateKey": base64.StdEncoding.EncodeToString(merchantDER),
		"platformPublicKey":  base64.StdEncoding.EncodeToString(platformDER),
		"notifyUrl":          "https://merchant.example/api/v1/payment/webhook/keyingpay",
		"returnUrl":          "https://merchant.example/payment/result",
	})
	require.NoError(t, err)
	require.Equal(t, keyingPayDefaultAPIBase, provider.config["apiBase"])
	require.Equal(t, map[string]string{"pid": "1001"}, provider.MerchantIdentityMetadata())

	_, err = NewKeyingPay("instance-1", map[string]string{
		"pid":                "0",
		"merchantPrivateKey": base64.StdEncoding.EncodeToString(merchantDER),
		"platformPublicKey":  base64.StdEncoding.EncodeToString(platformDER),
		"notifyUrl":          "https://merchant.example/notify",
		"returnUrl":          "https://merchant.example/result",
	})
	require.ErrorContains(t, err, "positive integer")
}

func TestCreateProviderRegistersKeyingPay(t *testing.T) {
	merchantKey, platformKey := keyingPayTestKeys(t)
	merchantDER, err := x509.MarshalPKCS8PrivateKey(merchantKey)
	require.NoError(t, err)
	platformDER, err := x509.MarshalPKIXPublicKey(&platformKey.PublicKey)
	require.NoError(t, err)

	created, err := CreateProvider(payment.TypeKeyingPay, "instance-factory", map[string]string{
		"pid":                "1001",
		"merchantPrivateKey": base64.StdEncoding.EncodeToString(merchantDER),
		"platformPublicKey":  base64.StdEncoding.EncodeToString(platformDER),
		"notifyUrl":          "https://merchant.example/api/v1/payment/webhook/keyingpay",
		"returnUrl":          "https://merchant.example/payment/result",
	})
	require.NoError(t, err)
	require.Equal(t, payment.TypeKeyingPay, created.ProviderKey())
	require.ElementsMatch(t, []payment.PaymentType{payment.TypeAlipay, payment.TypeWxpay}, created.SupportedTypes())
}

func TestKeyingPayCreatePaymentModes(t *testing.T) {
	tests := []struct {
		name        string
		paymentMode string
		payType     string
		payInfo     string
		wantPayURL  string
		wantQRCode  string
	}{
		{
			name:        "qrcode response remains usable on desktop and mobile",
			paymentMode: keyingPayPaymentModeQRCode,
			payType:     "qrcode",
			payInfo:     "https://pay.example/qr/123",
			wantPayURL:  "https://pay.example/qr/123",
			wantQRCode:  "https://pay.example/qr/123",
		},
		{
			name:        "popup uses jump url",
			paymentMode: "popup",
			payType:     "jump",
			payInfo:     "https://pay.example/cashier/123",
			wantPayURL:  "https://pay.example/cashier/123",
		},
		{
			name:        "qrcode mode can render signed jump url as qr",
			paymentMode: keyingPayPaymentModeQRCode,
			payType:     "jump",
			payInfo:     "https://pay.example/cashier/456",
			wantPayURL:  "https://pay.example/cashier/456",
			wantQRCode:  "https://pay.example/cashier/456",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			merchantKey, platformKey := keyingPayTestKeys(t)
			now := time.Unix(1_800_000_000, 0)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/pay/create", r.URL.Path)
				form := keyingPayTestVerifyRequest(t, r, &merchantKey.PublicKey)
				require.Equal(t, keyingPayMethodJump, form.Get("method"))
				require.Equal(t, "alipay", form.Get("type"))
				require.Equal(t, "sub2_order_1", form.Get("out_trade_no"))
				require.Equal(t, "12.34", form.Get("money"))
				keyingPayTestWriteSignedJSON(t, w, platformKey, now, map[string]any{
					"code":      0,
					"trade_no":  "KP20260001",
					"pay_type":  testCase.payType,
					"pay_info":  testCase.payInfo,
					"extension": "signed-extension",
				})
			}))
			defer server.Close()

			provider := keyingPayTestProvider(t, server.URL, testCase.paymentMode, merchantKey, platformKey, now)
			result, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
				OrderID:     "sub2_order_1",
				Amount:      "12.34",
				PaymentType: payment.TypeAlipay,
				Subject:     "Balance recharge",
				ClientIP:    "127.0.0.1",
			})
			require.NoError(t, err)
			require.Equal(t, "KP20260001", result.TradeNo)
			require.Equal(t, testCase.wantPayURL, result.PayURL)
			require.Equal(t, testCase.wantQRCode, result.QRCode)
		})
	}
}

func TestKeyingPayRejectsTamperedOrStaleResponse(t *testing.T) {
	tests := []struct {
		name       string
		responseAt func(time.Time) time.Time
		tamper     bool
		want       string
	}{
		{
			name:       "tampered signed response",
			responseAt: func(now time.Time) time.Time { return now },
			tamper:     true,
			want:       "invalid signature",
		},
		{
			name:       "stale signed response",
			responseAt: func(now time.Time) time.Time { return now.Add(-keyingPayTimestampTolerance - time.Second) },
			want:       "timestamp outside tolerance",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			merchantKey, platformKey := keyingPayTestKeys(t)
			now := time.Unix(1_800_000_000, 0)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				payload := map[string]any{
					"code":     0,
					"trade_no": "KP20260002",
					"pay_type": "jump",
					"pay_info": "https://pay.example/cashier/2",
				}
				keyingPayTestWriteSignedJSONWithTamper(t, w, platformKey, testCase.responseAt(now), payload, testCase.tamper)
			}))
			defer server.Close()

			provider := keyingPayTestProvider(t, server.URL, "popup", merchantKey, platformKey, now)
			_, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
				OrderID: "order-2", Amount: "1.00", PaymentType: payment.TypeWxpay,
				Subject: "Recharge", ClientIP: "127.0.0.1",
			})
			require.ErrorContains(t, err, testCase.want)
		})
	}
}

func TestKeyingPayVerifyNotificationIncludesExtensionFields(t *testing.T) {
	merchantKey, platformKey := keyingPayTestKeys(t)
	now := time.Unix(1_800_000_000, 0)
	provider := keyingPayTestProvider(t, "http://127.0.0.1:1", "popup", merchantKey, platformKey, now)
	values := url.Values{
		"pid":          {"1001"},
		"trade_no":     {"KP20260003"},
		"out_trade_no": {"sub2_order_3"},
		"type":         {"alipay"},
		"trade_status": {keyingPayTradeStatusSuccess},
		"money":        {"8.80"},
		"timestamp":    {strconv.FormatInt(now.Unix(), 10)},
		"sign_type":    {keyingPaySignType},
		"new_field":    {"future-compatible"},
	}
	signature, err := signKeyingPayValues(values, platformKey)
	require.NoError(t, err)
	values.Set("sign", signature)

	notification, err := provider.VerifyNotification(context.Background(), values.Encode(), nil)
	require.NoError(t, err)
	require.Equal(t, "sub2_order_3", notification.OrderID)
	require.Equal(t, "KP20260003", notification.TradeNo)
	require.Equal(t, 8.80, notification.Amount)
	require.Equal(t, "1001", notification.Metadata["pid"])

	values.Set("new_field", "tampered")
	_, err = provider.VerifyNotification(context.Background(), values.Encode(), nil)
	require.ErrorContains(t, err, "invalid signature")
}

func TestKeyingPayVerifyNotificationRejectsPIDAndTimestampMismatch(t *testing.T) {
	merchantKey, platformKey := keyingPayTestKeys(t)
	now := time.Unix(1_800_000_000, 0)
	provider := keyingPayTestProvider(t, "http://127.0.0.1:1", "popup", merchantKey, platformKey, now)

	for _, testCase := range []struct {
		name      string
		pid       string
		timestamp time.Time
		want      string
	}{
		{name: "pid mismatch", pid: "9999", timestamp: now, want: "pid mismatch"},
		{name: "stale timestamp", pid: "1001", timestamp: now.Add(-keyingPayTimestampTolerance - time.Second), want: "timestamp outside tolerance"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			values := url.Values{
				"pid":          {testCase.pid},
				"trade_no":     {"KP20260004"},
				"out_trade_no": {"sub2_order_4"},
				"trade_status": {keyingPayTradeStatusSuccess},
				"money":        {"1.00"},
				"timestamp":    {strconv.FormatInt(testCase.timestamp.Unix(), 10)},
				"sign_type":    {keyingPaySignType},
			}
			signature, err := signKeyingPayValues(values, platformKey)
			require.NoError(t, err)
			values.Set("sign", signature)
			_, err = provider.VerifyNotification(context.Background(), values.Encode(), nil)
			require.ErrorContains(t, err, testCase.want)
		})
	}
}

func TestKeyingPayQueryOrderStatuses(t *testing.T) {
	merchantKey, platformKey := keyingPayTestKeys(t)
	now := time.Unix(1_800_000_000, 0)
	statuses := []struct {
		upstream int
		want     string
	}{
		{upstream: 0, want: payment.ProviderStatusPending},
		{upstream: 1, want: payment.ProviderStatusPaid},
		{upstream: 2, want: payment.ProviderStatusRefunded},
		{upstream: 3, want: payment.ProviderStatusPending},
		{upstream: 4, want: payment.ProviderStatusPending},
	}

	for _, testCase := range statuses {
		t.Run(fmt.Sprintf("status_%d", testCase.upstream), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				form := keyingPayTestVerifyRequest(t, r, &merchantKey.PublicKey)
				require.Equal(t, "sub2_order_query", form.Get("out_trade_no"))
				keyingPayTestWriteSignedJSON(t, w, platformKey, now, map[string]any{
					"code":         0,
					"pid":          1001,
					"trade_no":     "KPQUERY01",
					"out_trade_no": "sub2_order_query",
					"status":       testCase.upstream,
					"money":        "22.50",
					"type":         "wxpay",
					"endtime":      "2026-07-11 12:34:56",
				})
			}))
			defer server.Close()
			provider := keyingPayTestProvider(t, server.URL, "popup", merchantKey, platformKey, now)
			result, err := provider.QueryOrder(context.Background(), "sub2_order_query")
			require.NoError(t, err)
			require.Equal(t, testCase.want, result.Status)
			require.Equal(t, 22.50, result.Amount)
			require.Equal(t, "KPQUERY01", result.TradeNo)
			require.Equal(t, "2026-07-11T12:34:56+08:00", result.PaidAt)
		})
	}
}

func TestKeyingPayRefundQueryAndClose(t *testing.T) {
	merchantKey, platformKey := keyingPayTestKeys(t)
	now := time.Unix(1_800_000_000, 0)
	var refundRequestIDs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := keyingPayTestVerifyRequest(t, r, &merchantKey.PublicKey)
		switch r.URL.Path {
		case "/api/pay/refund":
			refundRequestIDs = append(refundRequestIDs, form.Get("out_refund_no"))
			require.Equal(t, "KPTRADE05", form.Get("trade_no"))
			require.Equal(t, "5.50", form.Get("money"))
			keyingPayTestWriteSignedJSON(t, w, platformKey, now, map[string]any{
				"code":          0,
				"refund_no":     "KPRF05",
				"out_refund_no": form.Get("out_refund_no"),
				"trade_no":      "KPTRADE05",
				"money":         "5.50",
			})
		case "/api/pay/refundquery":
			require.Equal(t, "KPRF05", form.Get("refund_no"))
			keyingPayTestWriteSignedJSON(t, w, platformKey, now, map[string]any{
				"code":      0,
				"refund_no": "KPRF05",
				"status":    1,
			})
		case "/api/pay/close":
			require.Equal(t, "sub2_order_5", form.Get("out_trade_no"))
			keyingPayTestWriteSignedJSON(t, w, platformKey, now, map[string]any{"code": 0, "msg": "closed"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := keyingPayTestProvider(t, server.URL, "popup", merchantKey, platformKey, now)
	for range 2 {
		result, err := provider.Refund(context.Background(), payment.RefundRequest{
			TradeNo: "KPTRADE05", OrderID: "sub2_order_5", Amount: "5.50",
		})
		require.NoError(t, err)
		require.Equal(t, "KPRF05", result.RefundID)
		require.Equal(t, payment.ProviderStatusSuccess, result.Status)
	}
	require.Len(t, refundRequestIDs, 2)
	require.Equal(t, refundRequestIDs[0], refundRequestIDs[1])

	queryResult, err := provider.QueryRefund(context.Background(), payment.RefundQueryRequest{
		TradeNo: "KPTRADE05", OrderID: "sub2_order_5", RefundID: "KPRF05", Amount: "5.50",
	})
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusSuccess, queryResult.Status)
	require.NoError(t, provider.CancelPayment(context.Background(), "sub2_order_5"))
}

func keyingPayTestKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PrivateKey) {
	t.Helper()
	keyingPayTestKeysOnce.Do(func() {
		keyingPayMerchantKey, keyingPayTestKeysErr = rsa.GenerateKey(rand.Reader, 2048)
		if keyingPayTestKeysErr != nil {
			return
		}
		keyingPayPlatformKey, keyingPayTestKeysErr = rsa.GenerateKey(rand.Reader, 2048)
	})
	require.NoError(t, keyingPayTestKeysErr)
	return keyingPayMerchantKey, keyingPayPlatformKey
}

func keyingPayTestProvider(
	t *testing.T,
	apiBase string,
	paymentMode string,
	merchantKey *rsa.PrivateKey,
	platformKey *rsa.PrivateKey,
	now time.Time,
) *KeyingPay {
	t.Helper()
	merchantDER, err := x509.MarshalPKCS8PrivateKey(merchantKey)
	require.NoError(t, err)
	platformDER, err := x509.MarshalPKIXPublicKey(&platformKey.PublicKey)
	require.NoError(t, err)
	provider, err := NewKeyingPay("instance-test", map[string]string{
		"pid":                "1001",
		"merchantPrivateKey": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: merchantDER})),
		"platformPublicKey":  string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: platformDER})),
		"apiBase":            apiBase,
		"notifyUrl":          "https://merchant.example/api/v1/payment/webhook/keyingpay",
		"returnUrl":          "https://merchant.example/payment/result",
		"paymentMode":        paymentMode,
	})
	require.NoError(t, err)
	provider.now = func() time.Time { return now }
	return provider
}

func keyingPayTestVerifyRequest(t *testing.T, request *http.Request, publicKey *rsa.PublicKey) url.Values {
	t.Helper()
	require.Equal(t, http.MethodPost, request.Method)
	require.Equal(t, "application/x-www-form-urlencoded", request.Header.Get("Content-Type"))
	require.NoError(t, request.ParseForm())
	fields, err := strictKeyingPayFormFields(request.PostForm)
	require.NoError(t, err)
	require.Equal(t, keyingPaySignType, fields["sign_type"])
	require.NoError(t, verifyKeyingPaySignature(canonicalKeyingPayFields(fields), fields["sign"], publicKey))
	return request.PostForm
}

func keyingPayTestWriteSignedJSON(
	t *testing.T,
	w http.ResponseWriter,
	privateKey *rsa.PrivateKey,
	now time.Time,
	payload map[string]any,
) {
	t.Helper()
	keyingPayTestWriteSignedJSONWithTamper(t, w, privateKey, now, payload, false)
}

func keyingPayTestWriteSignedJSONWithTamper(
	t *testing.T,
	w http.ResponseWriter,
	privateKey *rsa.PrivateKey,
	now time.Time,
	payload map[string]any,
	tamper bool,
) {
	t.Helper()
	response := make(map[string]any, len(payload)+3)
	fields := make(map[string]string, len(payload)+3)
	for key, value := range payload {
		response[key] = value
		raw, err := json.Marshal(value)
		require.NoError(t, err)
		scalar, ok, err := keyingPayJSONScalar(raw)
		require.NoError(t, err)
		require.True(t, ok)
		fields[key] = scalar
	}
	response["timestamp"] = now.Unix()
	response["sign_type"] = keyingPaySignType
	fields["timestamp"] = strconv.FormatInt(now.Unix(), 10)
	fields["sign_type"] = keyingPaySignType
	signature, err := signKeyingPayCanonical(canonicalKeyingPayFields(fields), privateKey)
	require.NoError(t, err)
	response["sign"] = signature
	if tamper {
		response["pay_info"] = "https://attacker.example/tampered"
	}
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(response))
}
