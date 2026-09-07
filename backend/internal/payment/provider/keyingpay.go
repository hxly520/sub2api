package provider

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

const (
	keyingPayDefaultAPIBase      = "https://api.keyingpay.org"
	keyingPaySignType            = "RSA"
	keyingPayTradeStatusSuccess  = "TRADE_SUCCESS"
	keyingPayPaymentModeQRCode   = "qrcode"
	keyingPayMethodJump          = "jump"
	keyingPayHTTPTimeout         = 15 * time.Second
	keyingPayTimestampTolerance  = 5 * time.Minute
	keyingPayMaxResponseSize     = 1 << 20
	keyingPayMaxErrorSummarySize = 512
)

// KeyingPay implements the KeyingPay V2 SHA256WithRSA payment API.
type KeyingPay struct {
	instanceID         string
	config             map[string]string
	merchantPrivateKey *rsa.PrivateKey
	platformPublicKey  *rsa.PublicKey
	httpClient         *http.Client
	now                func() time.Time
}

// NewKeyingPay creates a KeyingPay V2 provider.
// Required config keys: pid, merchantPrivateKey, platformPublicKey, notifyUrl, returnUrl.
// apiBase defaults to https://api.keyingpay.org.
func NewKeyingPay(instanceID string, config map[string]string) (*KeyingPay, error) {
	cfg := make(map[string]string, len(config)+1)
	for key, value := range config {
		cfg[key] = strings.TrimSpace(value)
	}
	for _, key := range []string{"pid", "merchantPrivateKey", "platformPublicKey", "notifyUrl", "returnUrl"} {
		if cfg[key] == "" {
			return nil, fmt.Errorf("keyingpay config missing required key: %s", key)
		}
	}
	pid, err := strconv.ParseUint(cfg["pid"], 10, 64)
	if err != nil || pid == 0 {
		return nil, fmt.Errorf("keyingpay pid must be a positive integer")
	}

	apiBase, err := normalizeKeyingPayAPIBase(cfg["apiBase"])
	if err != nil {
		return nil, err
	}
	cfg["apiBase"] = apiBase
	for _, key := range []string{"notifyUrl", "returnUrl"} {
		if err := validateKeyingPayCallbackURL(cfg[key]); err != nil {
			return nil, fmt.Errorf("keyingpay %s is invalid: %w", key, err)
		}
	}

	privateKey, err := parseKeyingPayPrivateKey(cfg["merchantPrivateKey"])
	if err != nil {
		return nil, fmt.Errorf("keyingpay merchant private key is invalid: %w", err)
	}
	publicKey, err := parseKeyingPayPublicKey(cfg["platformPublicKey"])
	if err != nil {
		return nil, fmt.Errorf("keyingpay platform public key is invalid: %w", err)
	}

	return &KeyingPay{
		instanceID:         strings.TrimSpace(instanceID),
		config:             cfg,
		merchantPrivateKey: privateKey,
		platformPublicKey:  publicKey,
		httpClient: &http.Client{
			Timeout: keyingPayHTTPTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		now: time.Now,
	}, nil
}

func (k *KeyingPay) Name() string        { return "KeyingPay" }
func (k *KeyingPay) ProviderKey() string { return payment.TypeKeyingPay }
func (k *KeyingPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAlipay, payment.TypeWxpay}
}

func (k *KeyingPay) MerchantIdentityMetadata() map[string]string {
	if k == nil || strings.TrimSpace(k.config["pid"]) == "" {
		return nil
	}
	return map[string]string{"pid": strings.TrimSpace(k.config["pid"])}
}

func (k *KeyingPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	paymentType := payment.GetBasePaymentType(strings.TrimSpace(req.PaymentType))
	if paymentType != payment.TypeAlipay && paymentType != payment.TypeWxpay {
		return nil, fmt.Errorf("keyingpay unsupported payment type: %s", req.PaymentType)
	}
	if strings.TrimSpace(req.OrderID) == "" {
		return nil, fmt.Errorf("keyingpay create missing order id")
	}
	if err := validateKeyingPayAmount(req.Amount); err != nil {
		return nil, fmt.Errorf("keyingpay create invalid amount: %w", err)
	}
	clientIP := strings.TrimSpace(req.ClientIP)
	if net.ParseIP(clientIP) == nil {
		return nil, fmt.Errorf("keyingpay create invalid client ip")
	}

	notifyURL, returnURL := k.resolveURLs(req)
	if err := validateKeyingPayCallbackURL(notifyURL); err != nil {
		return nil, fmt.Errorf("keyingpay notify url is invalid: %w", err)
	}
	if err := validateKeyingPayCallbackURL(returnURL); err != nil {
		return nil, fmt.Errorf("keyingpay return url is invalid: %w", err)
	}

	fields, err := k.call(ctx, "/api/pay/create", map[string]string{
		"method":       keyingPayMethodJump,
		"type":         paymentType,
		"out_trade_no": strings.TrimSpace(req.OrderID),
		"notify_url":   notifyURL,
		"return_url":   returnURL,
		"name":         truncateKeyingPayUTF8(req.Subject, 127),
		"money":        strings.TrimSpace(req.Amount),
		"clientip":     clientIP,
	})
	if err != nil {
		return nil, fmt.Errorf("keyingpay create: %w", err)
	}
	if err := keyingPayRequireSuccess(fields); err != nil {
		return nil, fmt.Errorf("keyingpay create: %w", err)
	}

	tradeNo := strings.TrimSpace(fields["trade_no"])
	payType := strings.ToLower(strings.TrimSpace(fields["pay_type"]))
	payInfo := strings.TrimSpace(fields["pay_info"])
	if tradeNo == "" || payType == "" || payInfo == "" {
		return nil, fmt.Errorf("keyingpay create response missing payment fields")
	}

	result := &payment.CreatePaymentResponse{TradeNo: tradeNo}
	switch payType {
	case "jump", "scheme", "urlscheme":
		if err := validateKeyingPayPayURL(payInfo); err != nil {
			return nil, fmt.Errorf("keyingpay create returned invalid payment url: %w", err)
		}
		result.PayURL = payInfo
		if strings.EqualFold(k.config["paymentMode"], keyingPayPaymentModeQRCode) {
			result.QRCode = payInfo
		}
	case "qrcode":
		result.QRCode = payInfo
		if validateKeyingPayPayURL(payInfo) == nil {
			result.PayURL = payInfo
		}
	case "jsapi":
		var payload payment.WechatJSAPIPayload
		if err := json.Unmarshal([]byte(payInfo), &payload); err != nil {
			return nil, fmt.Errorf("keyingpay create returned invalid jsapi payload")
		}
		if payload.AppID == "" || payload.TimeStamp == "" || payload.NonceStr == "" || payload.Package == "" || payload.PaySign == "" {
			return nil, fmt.Errorf("keyingpay create returned incomplete jsapi payload")
		}
		result.ResultType = payment.CreatePaymentResultJSAPIReady
		result.JSAPI = &payload
	case "html", "app", "scan", "wxplugin", "wxapp":
		return nil, fmt.Errorf("keyingpay create returned unsupported pay_type: %s", payType)
	default:
		return nil, fmt.Errorf("keyingpay create returned unknown pay_type: %s", payType)
	}
	return result, nil
}

func (k *KeyingPay) QueryOrder(ctx context.Context, outTradeNo string) (*payment.QueryOrderResponse, error) {
	outTradeNo = strings.TrimSpace(outTradeNo)
	if outTradeNo == "" {
		return nil, fmt.Errorf("keyingpay query missing order id")
	}
	fields, err := k.call(ctx, "/api/pay/query", map[string]string{"out_trade_no": outTradeNo})
	if err != nil {
		return nil, fmt.Errorf("keyingpay query: %w", err)
	}
	if err := keyingPayRequireSuccess(fields); err != nil {
		return nil, fmt.Errorf("keyingpay query: %w", err)
	}
	if err := k.validateResponsePID(fields); err != nil {
		return nil, err
	}
	if returned := strings.TrimSpace(fields["out_trade_no"]); returned != "" && returned != outTradeNo {
		return nil, fmt.Errorf("keyingpay query order id mismatch")
	}

	statusCode, err := strconv.Atoi(strings.TrimSpace(fields["status"]))
	if err != nil {
		return nil, fmt.Errorf("keyingpay query returned invalid status")
	}
	var status string
	switch statusCode {
	case 0, 4:
		status = payment.ProviderStatusPending
	case 1:
		status = payment.ProviderStatusPaid
	case 2:
		status = payment.ProviderStatusRefunded
	case 3:
		// KeyingPay documents status 3 as frozen. Keep the local order pending:
		// a frozen order is not a definitive payment failure and may still be
		// resolved by the provider or an administrator.
		status = payment.ProviderStatusPending
	default:
		return nil, fmt.Errorf("keyingpay query returned unknown status: %d", statusCode)
	}

	amount, err := strconv.ParseFloat(strings.TrimSpace(fields["money"]), 64)
	if err != nil || amount < 0 {
		return nil, fmt.Errorf("keyingpay query returned invalid amount")
	}
	metadata := k.MerchantIdentityMetadata()
	metadata["status"] = strconv.Itoa(statusCode)
	if paymentType := strings.TrimSpace(fields["type"]); paymentType != "" {
		metadata["type"] = paymentType
	}
	return &payment.QueryOrderResponse{
		TradeNo:  strings.TrimSpace(fields["trade_no"]),
		Status:   status,
		Amount:   amount,
		PaidAt:   keyingPayRFC3339(fields["endtime"]),
		Metadata: metadata,
	}, nil
}

func (k *KeyingPay) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	values, err := url.ParseQuery(rawBody)
	if err != nil {
		return nil, fmt.Errorf("keyingpay notification parse failed")
	}
	fields, err := strictKeyingPayFormFields(values)
	if err != nil {
		return nil, err
	}
	if err := k.verifyFields(fields); err != nil {
		return nil, fmt.Errorf("keyingpay notification verification failed: %w", err)
	}
	if err := k.validateResponsePID(fields); err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(fields["trade_status"]), keyingPayTradeStatusSuccess) {
		return nil, nil
	}

	tradeNo := strings.TrimSpace(fields["trade_no"])
	outTradeNo := strings.TrimSpace(fields["out_trade_no"])
	if tradeNo == "" || outTradeNo == "" {
		return nil, fmt.Errorf("keyingpay notification missing order identifiers")
	}
	amount, err := strconv.ParseFloat(strings.TrimSpace(fields["money"]), 64)
	if err != nil || amount <= 0 {
		return nil, fmt.Errorf("keyingpay notification invalid amount")
	}
	metadata := k.MerchantIdentityMetadata()
	metadata["trade_status"] = keyingPayTradeStatusSuccess
	if paymentType := strings.TrimSpace(fields["type"]); paymentType != "" {
		metadata["type"] = paymentType
	}
	return &payment.PaymentNotification{
		TradeNo:  tradeNo,
		OrderID:  outTradeNo,
		Amount:   amount,
		Status:   payment.ProviderStatusSuccess,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

func (k *KeyingPay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	if err := validateKeyingPayAmount(req.Amount); err != nil {
		return nil, fmt.Errorf("keyingpay refund invalid amount: %w", err)
	}
	refundReference := strings.TrimSpace(req.OrderID)
	if refundReference == "" {
		refundReference = strings.TrimSpace(req.TradeNo)
	}
	params := map[string]string{
		"money":         strings.TrimSpace(req.Amount),
		"out_refund_no": k.refundRequestID(refundReference, req.Amount),
	}
	if tradeNo := strings.TrimSpace(req.TradeNo); tradeNo != "" {
		params["trade_no"] = tradeNo
	} else if orderID := strings.TrimSpace(req.OrderID); orderID != "" {
		params["out_trade_no"] = orderID
	} else {
		return nil, fmt.Errorf("keyingpay refund missing order identifier")
	}

	fields, err := k.call(ctx, "/api/pay/refund", params)
	if err != nil {
		return nil, fmt.Errorf("keyingpay refund: %w", err)
	}
	if err := keyingPayRequireSuccess(fields); err != nil {
		return nil, fmt.Errorf("keyingpay refund: %w", err)
	}
	refundNo := strings.TrimSpace(fields["refund_no"])
	if refundNo == "" {
		return nil, fmt.Errorf("keyingpay refund response missing refund number")
	}
	if returned := strings.TrimSpace(fields["out_refund_no"]); returned != "" && returned != params["out_refund_no"] {
		return nil, fmt.Errorf("keyingpay refund request id mismatch")
	}
	if returned := strings.TrimSpace(fields["money"]); returned != "" && !keyingPayAmountsEqual(returned, req.Amount) {
		return nil, fmt.Errorf("keyingpay refund amount mismatch")
	}
	return &payment.RefundResponse{RefundID: refundNo, Status: payment.ProviderStatusSuccess}, nil
}

func (k *KeyingPay) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	params := make(map[string]string, 1)
	if refundNo := strings.TrimSpace(req.RefundID); refundNo != "" {
		params["refund_no"] = refundNo
	} else {
		refundReference := strings.TrimSpace(req.OrderID)
		if refundReference == "" {
			refundReference = strings.TrimSpace(req.TradeNo)
		}
		params["out_refund_no"] = k.refundRequestID(refundReference, req.Amount)
	}
	fields, err := k.call(ctx, "/api/pay/refundquery", params)
	if err != nil {
		return nil, fmt.Errorf("keyingpay refund query: %w", err)
	}
	if err := keyingPayRequireSuccess(fields); err != nil {
		return nil, fmt.Errorf("keyingpay refund query: %w", err)
	}
	statusCode, err := strconv.Atoi(strings.TrimSpace(fields["status"]))
	if err != nil {
		return nil, fmt.Errorf("keyingpay refund query returned invalid status")
	}
	status := payment.ProviderStatusFailed
	if statusCode == 1 {
		status = payment.ProviderStatusSuccess
	} else if statusCode != 0 {
		return nil, fmt.Errorf("keyingpay refund query returned unknown status: %d", statusCode)
	}
	refundNo := strings.TrimSpace(fields["refund_no"])
	if refundNo == "" {
		refundNo = strings.TrimSpace(req.RefundID)
	}
	return &payment.RefundResponse{RefundID: refundNo, Status: status}, nil
}

func (k *KeyingPay) CancelPayment(ctx context.Context, outTradeNo string) error {
	outTradeNo = strings.TrimSpace(outTradeNo)
	if outTradeNo == "" {
		return fmt.Errorf("keyingpay close missing order id")
	}
	fields, err := k.call(ctx, "/api/pay/close", map[string]string{"out_trade_no": outTradeNo})
	if err != nil {
		return fmt.Errorf("keyingpay close: %w", err)
	}
	if err := keyingPayRequireSuccess(fields); err != nil {
		return fmt.Errorf("keyingpay close: %w", err)
	}
	return nil
}

func (k *KeyingPay) resolveURLs(req payment.CreatePaymentRequest) (string, string) {
	notifyURL := strings.TrimSpace(req.NotifyURL)
	if notifyURL == "" {
		notifyURL = strings.TrimSpace(k.config["notifyUrl"])
	}
	returnURL := strings.TrimSpace(req.ReturnURL)
	if returnURL == "" {
		returnURL = strings.TrimSpace(k.config["returnUrl"])
	}
	return notifyURL, returnURL
}

func (k *KeyingPay) call(ctx context.Context, path string, params map[string]string) (map[string]string, error) {
	values := make(url.Values, len(params)+4)
	for key, value := range params {
		if value = strings.TrimSpace(value); value != "" {
			values.Set(key, value)
		}
	}
	values.Set("pid", k.config["pid"])
	values.Set("timestamp", strconv.FormatInt(k.currentTime().Unix(), 10))
	values.Set("sign_type", keyingPaySignType)
	signature, err := signKeyingPayValues(values, k.merchantPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}
	values.Set("sign", signature)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.config["apiBase"]+path, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, keyingPayMaxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(body) > keyingPayMaxResponseSize {
		return nil, fmt.Errorf("response exceeds size limit")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	fields, err := decodeKeyingPayJSONFields(body)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response")
	}
	if err := k.verifyFields(fields); err != nil {
		return nil, fmt.Errorf("response verification failed: %w", err)
	}
	return fields, nil
}

func (k *KeyingPay) verifyFields(fields map[string]string) error {
	if !strings.EqualFold(strings.TrimSpace(fields["sign_type"]), keyingPaySignType) {
		return fmt.Errorf("invalid sign type")
	}
	signature := strings.TrimSpace(fields["sign"])
	if signature == "" {
		return fmt.Errorf("missing signature")
	}
	canonical := canonicalKeyingPayFields(fields)
	if err := verifyKeyingPaySignature(canonical, signature, k.platformPublicKey); err != nil {
		return fmt.Errorf("invalid signature")
	}
	timestamp, err := strconv.ParseInt(strings.TrimSpace(fields["timestamp"]), 10, 64)
	if err != nil || timestamp <= 0 {
		return fmt.Errorf("invalid timestamp")
	}
	delta := k.currentTime().Unix() - timestamp
	if delta < 0 {
		delta = -delta
	}
	if delta > int64(keyingPayTimestampTolerance/time.Second) {
		return fmt.Errorf("timestamp outside tolerance")
	}
	return nil
}

func (k *KeyingPay) validateResponsePID(fields map[string]string) error {
	pid := strings.TrimSpace(fields["pid"])
	if pid == "" {
		return fmt.Errorf("keyingpay response missing pid")
	}
	if pid != strings.TrimSpace(k.config["pid"]) {
		return fmt.Errorf("keyingpay pid mismatch")
	}
	return nil
}

func (k *KeyingPay) currentTime() time.Time {
	if k != nil && k.now != nil {
		return k.now()
	}
	return time.Now()
}

func (k *KeyingPay) refundRequestID(orderID, amount string) string {
	canonicalAmount := strings.TrimSpace(amount)
	if minor, err := payment.AmountToMinorUnit(canonicalAmount, payment.DefaultPaymentCurrency); err == nil {
		canonicalAmount = strconv.FormatInt(minor, 10)
	}
	payload := strings.Join([]string{k.instanceID, strings.TrimSpace(orderID), canonicalAmount}, "|")
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("sub2api_%x", sum[:16])
}

func keyingPayRequireSuccess(fields map[string]string) error {
	code, err := strconv.Atoi(strings.TrimSpace(fields["code"]))
	if err != nil {
		return fmt.Errorf("response missing status code")
	}
	if code == 0 {
		return nil
	}
	message := sanitizeKeyingPayMessage(fields["msg"])
	if message == "" {
		message = "request rejected"
	}
	return fmt.Errorf("request rejected (code %d): %s", code, message)
}

func signKeyingPayValues(values url.Values, privateKey *rsa.PrivateKey) (string, error) {
	fields, err := strictKeyingPayFormFields(values)
	if err != nil {
		return "", err
	}
	return signKeyingPayCanonical(canonicalKeyingPayFields(fields), privateKey)
}

func signKeyingPayCanonical(canonical string, privateKey *rsa.PrivateKey) (string, error) {
	if privateKey == nil {
		return "", fmt.Errorf("private key is missing")
	}
	digest := sha256.Sum256([]byte(canonical))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func verifyKeyingPaySignature(canonical, signature string, publicKey *rsa.PublicKey) error {
	if publicKey == nil {
		return fmt.Errorf("public key is missing")
	}
	signature = strings.ReplaceAll(strings.TrimSpace(signature), " ", "+")
	decoded, err := decodeKeyingPayBase64(signature)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(canonical))
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], decoded)
}

func canonicalKeyingPayFields(fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for key, value := range fields {
		if strings.EqualFold(key, "sign") || strings.EqualFold(key, "sign_type") || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+fields[key])
	}
	return strings.Join(parts, "&")
}

func strictKeyingPayFormFields(values url.Values) (map[string]string, error) {
	fields := make(map[string]string, len(values))
	for key, entries := range values {
		if len(entries) != 1 {
			return nil, fmt.Errorf("keyingpay duplicate parameter: %s", key)
		}
		fields[key] = entries[0]
	}
	return fields, nil
}

func decodeKeyingPayJSONFields(body []byte) (map[string]string, error) {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("expected JSON object")
	}
	fields := make(map[string]string)
	seen := make(map[string]struct{})
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("invalid JSON key")
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate JSON key")
		}
		seen[key] = struct{}{}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		value, scalar, err := keyingPayJSONScalar(raw)
		if err != nil {
			return nil, err
		}
		if scalar {
			fields[key] = value
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON data")
	}
	return fields, nil
}

func keyingPayJSONScalar(raw json.RawMessage) (string, bool, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", false, nil
	}
	switch trimmed[0] {
	case '"':
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", false, err
		}
		return value, true, nil
	case '{', '[':
		return "", false, nil
	default:
		if trimmed == "true" || trimmed == "false" {
			return trimmed, true, nil
		}
		if _, err := strconv.ParseFloat(trimmed, 64); err != nil {
			return "", false, err
		}
		return trimmed, true, nil
	}
}

func normalizeKeyingPayAPIBase(raw string) (string, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		base = keyingPayDefaultAPIBase
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("keyingpay apiBase must be an absolute URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("keyingpay apiBase must not contain credentials, query, or fragment")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !keyingPayLoopbackHost(parsed.Hostname())) {
		return "", fmt.Errorf("keyingpay apiBase must use HTTPS")
	}
	path := strings.TrimRight(parsed.Path, "/")
	for _, suffix := range []string{"/api/pay/create", "/api/pay/query", "/api/pay/refund", "/api/pay/refundquery", "/api/pay/close", "/api/pay/submit"} {
		if strings.HasSuffix(strings.ToLower(path), suffix) {
			path = strings.TrimRight(path[:len(path)-len(suffix)], "/")
			break
		}
	}
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validateKeyingPayCallbackURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("must be an absolute URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("must not contain credentials")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && (scheme != "http" || !keyingPayLoopbackHost(parsed.Hostname())) {
		return fmt.Errorf("must use HTTPS")
	}
	return nil
}

func validateKeyingPayPayURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" {
		return fmt.Errorf("missing URL scheme")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https", "http", "weixin", "alipays", "intent":
		return nil
	default:
		return fmt.Errorf("unsupported URL scheme")
	}
}

func keyingPayLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateKeyingPayAmount(amount string) error {
	minor, err := payment.AmountToMinorUnit(strings.TrimSpace(amount), payment.DefaultPaymentCurrency)
	if err != nil {
		return err
	}
	if minor <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	return nil
}

func keyingPayAmountsEqual(left, right string) bool {
	leftMinor, leftErr := payment.AmountToMinorUnit(left, payment.DefaultPaymentCurrency)
	rightMinor, rightErr := payment.AmountToMinorUnit(right, payment.DefaultPaymentCurrency)
	return leftErr == nil && rightErr == nil && leftMinor == rightMinor
}

func keyingPayRFC3339(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.FixedZone("CST", 8*60*60)); err == nil {
		return parsed.Format(time.RFC3339)
	}
	return ""
}

func truncateKeyingPayUTF8(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func sanitizeKeyingPayMessage(message string) string {
	message = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, strings.TrimSpace(message))
	if len(message) > keyingPayMaxErrorSummarySize {
		message = truncateKeyingPayUTF8(message, keyingPayMaxErrorSummarySize) + "..."
	}
	return message
}

func parseKeyingPayPrivateKey(raw string) (*rsa.PrivateKey, error) {
	der, err := keyingPayDER(raw)
	if err != nil {
		return nil, err
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		if err := validateKeyingPayPrivateKey(key); err != nil {
			return nil, err
		}
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("unsupported private key format")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	if err := validateKeyingPayPrivateKey(key); err != nil {
		return nil, err
	}
	return key, nil
}

func validateKeyingPayPrivateKey(key *rsa.PrivateKey) error {
	if key == nil || key.N == nil || key.N.BitLen() < 2048 {
		return fmt.Errorf("RSA key must be at least 2048 bits")
	}
	if err := key.Validate(); err != nil {
		return fmt.Errorf("RSA key validation failed")
	}
	return nil
}

func parseKeyingPayPublicKey(raw string) (*rsa.PublicKey, error) {
	der, err := keyingPayDER(raw)
	if err != nil {
		return nil, err
	}
	if cert, err := x509.ParseCertificate(der); err == nil {
		if key, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			return validateKeyingPayPublicKey(key)
		}
	}
	if key, err := x509.ParsePKCS1PublicKey(der); err == nil {
		return validateKeyingPayPublicKey(key)
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("unsupported public key format")
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}
	return validateKeyingPayPublicKey(key)
}

func validateKeyingPayPublicKey(key *rsa.PublicKey) (*rsa.PublicKey, error) {
	if key == nil || key.N == nil || key.N.BitLen() < 2048 || key.E < 3 {
		return nil, fmt.Errorf("RSA key must be at least 2048 bits")
	}
	return key, nil
}

func keyingPayDER(raw string) ([]byte, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\n`, "\n"))
	if block, _ := pem.Decode([]byte(raw)); block != nil {
		return block.Bytes, nil
	}
	compact := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, raw)
	der, err := decodeKeyingPayBase64(compact)
	if err != nil {
		return nil, fmt.Errorf("key is neither PEM nor base64 DER")
	}
	return der, nil
}

func decodeKeyingPayBase64(raw string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if decoded, err := encoding.DecodeString(raw); err == nil {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("invalid base64")
}

var _ payment.Provider = (*KeyingPay)(nil)
var _ payment.RefundQueryProvider = (*KeyingPay)(nil)
var _ payment.CancelableProvider = (*KeyingPay)(nil)
var _ payment.MerchantIdentityProvider = (*KeyingPay)(nil)
