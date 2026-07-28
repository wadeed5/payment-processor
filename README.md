# Solution Architecture

## Problem statement
# Business flow problem reported

- Balance not reconciling after load test
- Txn amount debit from Account A but did not credit to Account B
- open question: txn retried getting processed twice, needs to be investigated

## Design flaws 

# Lack of DB transactions usage is making the system unstable
The credit and debit in the Transfer flow are two separate independant db calls that can make the ledger 
imbalance.

Example: 
Client A => balance 100 USD 
Client B => balance 80 USD

Client A wants to transfer 50 USD to Client B
Code flow: 
store.Transfer func is executed and Client A wallet is debit by 50 USD 
but the db call to credit client B fails due to any reason (db connnection timeout etc)
now we have a scenario in which Client A was debit by 50 and Client B was not credit 50 and there is no record of this transaction happening as failed txn was not recorded it ledger (for reconciliation)

