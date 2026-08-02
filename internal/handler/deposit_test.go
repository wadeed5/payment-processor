package handler

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHandleDeposit_PublishesCompletedAndCreditsWallet(t *testing.T) {
	env := setupHandlerTestEnv(t)

	const requestID = "50000000-0000-0000-0000-000000000001"
	payload, _ := json.Marshal(DepositRequest{
		RequestID: requestID,
		WalletID:  testWalletA,
		Amount:    100,
		Currency:  "USD",
	})

	if err := env.client.Conn.Publish("wallet.deposit", payload); err != nil {
		t.Fatalf("publish deposit: %v", err)
	}

	event := env.waitForEvent(t, "wallet.events.completed")
	if event.RequestID != requestID {
		t.Fatalf("event request_id = %q, want %q", event.RequestID, requestID)
	}
	if event.Operation != "deposit" {
		t.Fatalf("event operation = %q, want deposit", event.Operation)
	}
	if event.Status != "completed" {
		t.Fatalf("event status = %q, want completed", event.Status)
	}

	if balance := env.walletBalance(t, testWalletA); balance != 100 {
		t.Fatalf("wallet balance = %v, want 100", balance)
	}
}

func TestHandleDeposit_DuplicateRequestIsIdempotent(t *testing.T) {
	env := setupHandlerTestEnv(t)

	const requestID = "50000000-0000-0000-0000-000000000002"
	payload, _ := json.Marshal(DepositRequest{
		RequestID: requestID,
		WalletID:  testWalletA,
		Amount:    100,
		Currency:  "USD",
	})

	if err := env.client.Conn.Publish("wallet.deposit", payload); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	env.waitForEvent(t, "wallet.events.completed")

	if err := env.client.Conn.Publish("wallet.deposit", payload); err != nil {
		t.Fatalf("duplicate publish: %v", err)
	}
	dupEvent := env.waitForEvent(t, "wallet.events.completed")
	if dupEvent.Status != "completed" {
		t.Fatalf("duplicate event status = %q, want completed", dupEvent.Status)
	}

	if balance := env.walletBalance(t, testWalletA); balance != 100 {
		t.Fatalf("wallet balance = %v, want 100 after duplicate deposit", balance)
	}
}

func TestHandleDeposit_InvalidPayloadDoesNotPublishEvent(t *testing.T) {
	env := setupHandlerTestEnv(t)

	sub, err := env.conn.SubscribeSync("wallet.events.>")
	if err != nil {
		t.Fatalf("subscribe events: %v", err)
	}

	if err := env.client.Conn.Publish("wallet.deposit", []byte("{invalid")); err != nil {
		t.Fatalf("publish invalid payload: %v", err)
	}

	if _, err := sub.NextMsg(500 * time.Millisecond); err == nil {
		t.Fatal("expected no event for invalid payload")
	}
}
