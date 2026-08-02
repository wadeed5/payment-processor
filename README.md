# Solution Architecture

## Problem statement
### Business flow problem reported

- Balance not reconciling after load test
- Txn amount debit from Account A but did not credit to Account B
- open question: txn retried getting processed twice, needs to be investigated

## System gaps

### Lack of DB transactions usage is making the system unstable
The credit and debit in the Transfer flow are two separate independant db calls that can make the ledger 
imbalance.

Scenario 1: 
Client A => balance 100 USD 
Client B => balance 80 USD

Client A wants to transfer 50 USD to Client B
Code flow: 
store.Transfer func is executed and Client A wallet is debit by 50 USD 
but the db call to credit client B fails due to any reason (db connnection timeout etc)
now we have a scenario in which Client A was debit by 50 and Client B was not credit 50 and there is no record of this transaction happening as failed txn was not recorded it ledger (for reconciliation)

Scenario 2: 
For withdrawal and transfer the balance check is independent from actual db updated which will cause 
concurrency issue.
Wallet has 100 USD 
Two withdrawal for 60 USD comes (load test, duplicate txn scenario)
Both will read balance as 100 USD and pass the check which would result in account going in negative balance. 

### Balance being reported inconsistent
The balance is being reported by cummulating the ledger record (transactions) instead of the balance retrieved from wallet. 

total withdrawal are deducted from total deposit and remaining is reported back as balance.

Problems with reporting balance from the transaction ledger. 
- The scenario reported above in the db transaction scenario will through off the balance as client A was deducted but was not recorded in the ledger so it will throw off the amount reported as balance as it will
report more then what is there in the client A wallet. 

- Fetching transaction record not using the status column. 
This is more of a concern related to the correctness of the system, as status = 'Completed' should be used. 

- Deposit, Withdrawal, Transfer successful but the insert in transaction ledger failed will cause reporting of wrong balance for the client. 

### Publish failed on NATS in case of failure without recording a failed transaction
- This is also one of the issues that contributes to lack of correctness of the system, when an operation 
fails we are not recording a failed transaction in the ledger which would hurt reconciliation as reconciliation team will see a failed event on NATS but no record in the database, this is most concerning in the case on transfer as explained above. 

### Publish Completed on NATS before the transaction is record 
- This majorly contributes to the correctioness of the system as we publish completed on nats before we record a transaction, so in case if the recording of transaction failed we will have gaps in reconcilation data when on NATS the operation has succeeded but there is no record in the transaction ledger.

### lack of Idempotency
- The system in general is not having any kind of idempotency check neither on NATS level or any unique constraint in DB to ensure idempotency. Due to which the system will always process duplicate request if possible. 

### Lack on any type of validation (request body, account existence)
- The system lacks any kind of formal validation, there is no request validator that can enforce sanitization of request. In case of there is error in unmarshalling there is no publish error which will result in having caller thinking its still processing even though the request has failed.
 The operation lacks check for account existence which can lead to problems like:
- Same account transfer 
- Ghost account transfer


## Solution and fixes 
### Introduced DB level transactions. 
- All the interations with database have been wrapped in db transaction to be atomic, that removes all the inconsistency in data that gets introduced when 1 operation passes and the other fails.

### Balance is now reported via wallet balance
- The balance is now being reported via the wallet balance that will provide the high consistent result for the client, as ledger is better used for reconciliation and will be slower to process transaction and cumulate balance as the the record base grows. 

### The system has been made more consistent
- With DB level transaction and error handling fix the system is now less prune to breaking down when an error occurs. 

### Include of Db constraint to ensure idempotency on db level
- Added unique contraint using (request_id) on transction table and check that before proceeding with the operation to ensure that the system behaviour is consistent and no duplicate operation gets processed. 

### Recommended Fixes (not implemented)
- Checks to be implement on account existence before proceeding with operation.
- Use of currency to convert and process operation. (assuming right now that the system only supports 1 currency).
- NATS level enchancement making streams durable, supporting acknowledgements, delivery guarantees and error handling on subscribe functionality. 
- General performance improvment by implementing database indexes on fields that are freqently use to fetch data like to_wallet, from_wallet, request_id etc. 

## Testing

### Running and testing the services via NATS CLI

**1. Start services**

```bash
docker compose up -d
```

**2. Install NATS CLI (macOS)**

```bash
brew install nats-io/nats-tools/nats
```

**3. Subscribe to outcome events (optional, separate terminal)**

```bash
nats --server nats://localhost:4222 sub "wallet.events.>"
```

**4. Deposit funds**

```bash
nats --server nats://localhost:4222 pub wallet.deposit '{
  "request_id": "11111111-1111-1111-1111-111111111111",
  "wallet_id": "22222222-2222-2222-2222-222222222222",
  "amount": 100,
  "currency": "USD"
}'
```

Expected: `Published ... bytes to "wallet.deposit"`

**5. Check balance (request-reply)**

```bash
nats --server nats://localhost:4222 request wallet.balance '{
  "wallet_id": "22222222-2222-2222-2222-222222222222"
}'
```

Expected response:

```json
{"wallet_id":"22222222-2222-2222-2222-222222222222","balance":100,"currency":"USD"}
```

**6. Withdraw**

```bash
nats --server nats://localhost:4222 pub wallet.withdraw '{
  "request_id": "33333333-3333-3333-3333-333333333333",
  "wallet_id": "22222222-2222-2222-2222-222222222222",
  "amount": 30,
  "currency": "USD"
}'
```

**7. Transfer**

```bash
nats --server nats://localhost:4222 pub wallet.transfer '{
  "request_id": "44444444-4444-4444-4444-444444444444",
  "from_wallet_id": "22222222-2222-2222-2222-222222222222",
  "to_wallet_id": "55555555-5555-5555-5555-555555555555",
  "amount": 20,
  "currency": "USD"
}'
```

Use a **new `request_id`** for each operation (idempotency).

### Automated tests

```bash
docker compose up -d postgres
go test ./... -v
```

### Load test script

```bash
docker compose up -d postgres nats
docker compose stop wallet-service   # avoid duplicate handlers

./scripts/loadtest.sh
# or: go run ./cmd/loadtest -ops 200
```

## Test evidence

### NATS manual testing

**Deposit**

```text
$ nats --server nats://localhost:4222 pub wallet.deposit '{
  "request_id": "11111111-1111-1111-1111-111111111111",
  "wallet_id": "22222222-2222-2222-2222-222222222222",
  "amount": 100,
  "currency": "USD"
}'
Published 151 bytes to "wallet.deposit"
```

**Balance check**

```text
$ nats --server nats://localhost:4222 request wallet.balance '{
  "wallet_id": "22222222-2222-2222-2222-222222222222"
}'
Sending request on "wallet.balance"
Received with rtt 2.290084ms
{"wallet_id":"22222222-2222-2222-2222-222222222222","balance":100,"currency":"USD"}
```

**Event subscription**

```bash
nats --server nats://localhost:4222 sub "wallet.events.>"
```

### Load test (`go run ./cmd/loadtest`)

```text
=== wallet load test ===
NATS: nats://localhost:4222
Postgres: postgres://walletuser:walletpass@localhost:5432/wallet?sslmode=disable
Accounts: 3 | Initial balance each: 10000.00 | Random ops: 200

==> funding accounts
    account 1: 759d09d1-308c-4796-b7d2-8b29ceab9f04 -> 10000.00 USD
    account 2: 585474b1-5716-4014-9990-6e46a6f5d004 -> 10000.00 USD
    account 3: 6586443c-40ea-4014-9c1e-97f35404a5db -> 10000.00 USD

==> running 200 random operations
    completed 50/200 operations
    completed 100/200 operations
    completed 150/200 operations
    completed 200/200 operations

==> operation summary
    deposits : 61
    withdraws: 52 (includes expected insufficient-fund failures)
    transfers: 41 (includes expected insufficient-fund failures)
    balances : 49
    failed   : 0

==> final account report
    account 1: 759d09d1-308c-4796-b7d2-8b29ceab9f04
      wallet table : 8964.0000 USD
      ledger sum   : 8964.0000
      nats balance : 8964.0000 USD
      wallet vs ledger: OK
    account 2: 585474b1-5716-4014-9990-6e46a6f5d004
      wallet table : 10237.0000 USD
      ledger sum   : 10237.0000
      nats balance : 10237.0000 USD
      wallet vs ledger: OK
    account 3: 6586443c-40ea-4014-9c1e-97f35404a5db
      wallet table : 9967.0000 USD
      ledger sum   : 9967.0000
      nats balance : 9967.0000 USD
      wallet vs ledger: OK

==> result
    no discrepancies detected

==> cleaning up test data...
    all wallets and transactions deleted
```

## Summary: problems, fixes, and impact

The table below maps each identified issue to the change shipped (or deliberately deferred) and the business impact.

| Problem identified | How it was fixed | Impact of problem / fix |
|---|---|---|
| **Non-atomic transfer** — debit and credit were separate DB calls; sender could be debited without receiver being credited | Wrapped all wallet + ledger writes in a single DB transaction via `withTransaction()` in [`internal/storage/postgres.go`](internal/storage/postgres.go) | **Problem:** Money could leave Account A and never arrive in Account B; no atomic rollback. **Fix:** Transfer, deposit, and withdraw now commit or roll back together. |
| **Balance API used ledger sum** instead of `wallets.balance` | [`internal/handler/balance.go`](internal/handler/balance.go) now returns `GetWalletBalance()` directly | **Problem:** Reported balance could diverge from spendable balance when ledger was incomplete or wrong. **Fix:** Clients see consistent, authoritative wallet balance. |
| **No idempotency** — duplicate NATS messages with same `request_id` could double-apply | Added `UNIQUE (request_id)` in [`init.sql`](init.sql) and `tryClaimRequest()` with `ON CONFLICT DO NOTHING` before wallet mutation | **Problem:** Retries/load tests could credit or debit twice. **Fix:** Duplicate requests are safely ignored; handler republishes `completed` without changing balance. |
| **Wallet update and ledger insert were not atomic** — ledger could fail after wallet changed | Ledger insert moved inside the same DB transaction as wallet update (`recordTransactionWithTx` → `tryClaimRequest`) | **Problem:** Reconciliation gaps; balance handler (when ledger-based) showed wrong values. **Fix:** Wallet and ledger stay in sync for successful operations. |
| **`publishCompleted` before durable DB write** | Handlers call `store.Deposit/Withdraw/Transfer` first; events published only after storage succeeds | **Problem:** Clients could believe operation succeeded while DB state was incomplete. **Fix:** Event order matches committed state. |
| **Ledger balance query ignored `status` filter semantics** | `SumBalanceFromTransactions` now filters with `(from_wallet OR to_wallet) AND status = 'completed'` | **Problem:** Failed or invalid rows could skew reconciliation sums. **Fix:** Ledger sum reflects completed transactions only (used for reconciliation / load test). |
| **Concurrency race on withdraw/transfer** — check-then-act without row lock | **Not fixed** (documented under Recommended Fixes) | **Problem:** Under load, two concurrent withdraws can both pass balance check and overdraw. **Fix:** Deferred — needs `SELECT ... FOR UPDATE` or atomic conditional UPDATE. |
| **No failed transaction rows in ledger** | **Not fixed** | **Problem:** NATS `failed` events have no DB audit trail for reconciliation. **Fix:** Deferred — would require recording `status = 'failed'` rows on failure paths. |
| **No input / account validation** (bad JSON, ghost receiver wallet, self-transfer) | **Not fixed** (documented under Recommended Fixes) | **Problem:** Invalid requests fail silently or can debit sender when receiver does not exist. **Fix:** Deferred — needs validation layer and `RowsAffected` checks on transfer credit. |
| **No DB indexes on hot columns** | **Not fixed** | **Problem:** Ledger queries slow as transaction volume grows (load test timeouts → client retries). **Fix:** Deferred — index `request_id`, `from_wallet`, `to_wallet`. |

## Note: AI usage disclosure

- AI was used to format this README.
- AI was used for repetitive tasks such as test case generation and validation.
