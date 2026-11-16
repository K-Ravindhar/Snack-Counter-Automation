# Snack Counter Automation (Go)

A small runnable implementation of the snack refill lifecycle with simulated chat and HR integrations.

## Requirements
- Go 1.22+

## Run the demo
```bash
go run ./cmd
```

## Run tests
```bash
go test ./...
```

## What is implemented
- Fair assignment to available volunteers currently at the relevant office.
- Atomic assignment operations protected by a mutex.
- 24-hour task expiry and reassignment to a different volunteer.
- Office-specific monthly budget checks in NOK and INR.
- Purchase/receipt recording and monthly report totals without mixing currencies.
- Simulated chat and HR adapters.
- Payroll-currency reimbursement boundary via an FX interface. The included converter supports same-currency claims only; cross-currency claims require a real/test FX rate provider.

## Scope and limitations
The repository is in-memory for a focused exercise, so data is lost on restart and it is not safe for multiple service instances. In production, replace it with a transactional database and make HR submission idempotent using a durable outbox/idempotency key. HR status updates would arrive through a webhook or scheduled polling job. ReceiptURL is a stand-in for the receipt image/file reference expected by the HR API.
