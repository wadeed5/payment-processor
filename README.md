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