package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestPointsBridgeRepositoryAppliesCreditAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	transactionID := uuid.MustParse("b754d48e-2bb3-4d61-9428-e8de88c18670")
	input := service.PointsBalanceCreditInput{
		TransactionID: transactionID, UserID: 42, Amount: decimal.RequireFromString("0.05"),
		Kind: "checkin", SourceReference: "checkin:42:2026-07-29:1", Reason: "daily check-in",
		Nonce: transactionID.String(), PayloadHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RequestID: "request-1",
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO points_balance_credits").
		WithArgs(transactionID, int64(42), "0.05", "checkin", input.SourceReference, input.Reason,
			input.Nonce, input.PayloadHash, input.RequestID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("UPDATE users").WithArgs("0.05", int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow("12.34"))
	mock.ExpectExec("UPDATE points_balance_credits").WithArgs(transactionID, "12.34").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_logs").WithArgs(sqlmock.AnyArg(), input.RequestID, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := NewPointsBridgeRepository(db)
	result, err := repo.ApplyPointsBalanceCredit(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.TransactionID != transactionID || !result.BalanceAfter.Equal(decimal.RequireFromString("12.34")) || result.Idempotent {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPointsBridgeRepositoryReturnsOriginalIdempotentResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	transactionID := uuid.MustParse("b754d48e-2bb3-4d61-9428-e8de88c18670")
	input := service.PointsBalanceCreditInput{
		TransactionID: transactionID, UserID: 42, Amount: decimal.RequireFromString("0.05"),
		Kind: "checkin", SourceReference: "checkin:42:2026-07-29:1", Reason: "daily check-in",
		Nonce: transactionID.String(), PayloadHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO points_balance_credits").
		WithArgs(transactionID, int64(42), "0.05", "checkin", input.SourceReference, input.Reason,
			input.Nonce, input.PayloadHash, input.RequestID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT user_id, amount::text").WithArgs(transactionID).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "amount", "kind", "source_reference", "payload_hash", "balance_after",
		}).AddRow(int64(42), "0.05", "checkin", input.SourceReference, input.PayloadHash, "12.34"))
	mock.ExpectCommit()

	repo := NewPointsBridgeRepository(db)
	result, err := repo.ApplyPointsBalanceCredit(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Idempotent || !result.BalanceAfter.Equal(decimal.RequireFromString("12.34")) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
