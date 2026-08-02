package storage

import (
	"os"
	"testing"
)

const (
	testWalletA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	testWalletB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	testWalletC = "cccccccc-cccc-cccc-cccc-cccccccccccc"
)

func testPGURL() string {
	if url := os.Getenv("PG_URL"); url != "" {
		return url
	}
	return "postgres://walletuser:walletpass@localhost:5432/wallet?sslmode=disable"
}

func setupTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := NewStore(testPGURL())
	if err != nil {
		t.Skipf("postgres unavailable (start docker-compose): %v", err)
	}

	resetTables(t, store)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func resetTables(t *testing.T, store *Store) {
	t.Helper()

	if _, err := store.DB.Exec(`TRUNCATE transactions RESTART IDENTITY`); err != nil {
		t.Fatalf("truncate transactions: %v", err)
	}
	if _, err := store.DB.Exec(`TRUNCATE wallets`); err != nil {
		t.Fatalf("truncate wallets: %v", err)
	}
}

func mustDeposit(t *testing.T, store *Store, requestID, walletID string, amount float64) {
	t.Helper()

	if err := store.Deposit(requestID, walletID, amount); err != nil {
		t.Fatalf("deposit %s: %v", requestID, err)
	}
}

func countTransactions(t *testing.T, store *Store) int {
	t.Helper()

	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&count); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	return count
}

func countTransactionsByRequestID(t *testing.T, store *Store, requestID string) int {
	t.Helper()

	var count int
	err := store.DB.QueryRow(`SELECT COUNT(*) FROM transactions WHERE request_id = $1`, requestID).Scan(&count)
	if err != nil {
		t.Fatalf("count transactions by request_id: %v", err)
	}
	return count
}
