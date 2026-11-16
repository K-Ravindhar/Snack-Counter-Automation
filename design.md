# Snack Counter Automation — Design

## Goal and assumptions
Build one lightweight workflow for Oslo and Bengaluru that makes refill ownership explicit, prevents duplicate assignments, protects monthly budgets and gives management a reliable monthly view. Volunteers are registered employees. A volunteer is eligible only when marked available and currently located at the office needing the refill; they can update their current office when visiting. Each office spends in its local currency (Oslo: NOK 6,000/month; Bengaluru: INR 15,000/month). A task assignment expires after 24 hours. Pending purchases and submitted reimbursements count against the monthly budget because the money has already been spent. No exchange rate is supplied, so reporting never adds NOK and INR together.

## Refill lifecycle
1. An employee reports an empty snack box in the internal chat workflow (or through a small API). The service creates one `OPEN` refill.
2. The service selects an available volunteer physically at that office. It uses a fair ordering: fewest prior assignments, then oldest last-assigned time, then employee ID. The selected volunteer receives a direct chat notification and the task becomes `ASSIGNED` for 24 hours. If nobody is eligible, the task stays open and the office channel is notified.
3. The assigned volunteer buys the snacks and records quantity, amount and receipt. The service validates that the caller owns the assignment and that the purchase will not exceed the office's remaining monthly budget. It records the purchase in the office currency and moves the refill to `PURCHASED`.
4. The service converts the purchase amount to the volunteer's payroll currency through an FX interface, then submits employee ID, amount, payroll currency, description and receipt image/link to the HR system. On success it stores the HR claim ID and moves the refill to `SUBMITTED`.
5. HR approval remains a human/third-party process. A webhook or scheduled status sync can move the refill to `REIMBURSED` when HR confirms payment.
6. A background worker checks expired assignments. It excludes volunteers who already had that task and assigns the next eligible volunteer. If none is available, the task returns to `OPEN` and is broadcast again.

## Components and data
- **Chat adapter:** sends direct messages and office-channel notifications. Production version uses the company's chat API; this project includes a mock.
- **Refill service:** owns lifecycle transitions, assignment fairness and timeout/reassignment rules.
- **Volunteer data:** employee ID, name, home/current office, payroll currency, availability, assignment count and last-assigned time.
- **Refill data:** ID, office, snack, status, timestamps/deadline, assigned employee and volunteers already tried.
- **Purchase/reimbursement data:** purchase amount and currency, quantity, receipt reference, purchase date, HR claim ID, converted claim amount and payroll currency.
- **Budget/report service:** fixed monthly budget per office; sums purchases by purchase month and office currency. Management gets one report with office totals and currency totals.
- **HR adapter and FX adapter:** narrow interfaces isolate vendor-specific API details and currency conversion. The included HR and chat adapters are simulated.

## Preventing the current problems
- **Empty boxes ignored:** every empty report becomes a visible task with a named owner, deadline and escalation/reassignment.
- **Duplicate buyers:** assignment is an atomic state transition under a lock in this demo; a production database would use a transaction and conditional update/row lock plus a unique active assignment constraint. Purchase submission is accepted only from the assigned volunteer.
- **Unequal volunteer load:** least-assigned-first selection and availability controls spread work fairly. Employees can mark themselves unavailable or update their current office while travelling.
- **Slow reimbursement visibility:** the HR claim ID and status are stored; notifications can be sent on submission/approval/rejection. HR approval time itself cannot be controlled by this service.
- **Unknown spend:** each purchase is recorded once and included in monthly reporting. The budget check and purchase record happen atomically.
- **Cross-currency accuracy:** budgets are enforced in local office currency. The report presents NOK and INR separately rather than summing unlike currencies. Payroll reimbursement conversion uses an explicit FX adapter and stores both original and converted amounts.

## Trade-offs and deliberate non-goals
This exercise uses an in-memory, mutex-protected store to keep the core rules runnable without infrastructure. It is not durable across restarts and is not a multi-instance deployment solution; production would use PostgreSQL transactions, migrations, audit history and idempotency keys. Chat, HR and FX are interfaces with mock implementations. I would not build a mobile app, inventory forecasting, barcode scanning, complex approval chains or snack-level consumption analytics in the first iteration. I would add those only after usage data shows they are needed. HR approval remains with HR; the automation only submits and tracks claims.
