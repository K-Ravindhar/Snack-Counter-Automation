package models

import "time"

type Office string
type Currency string
type RefillStatus string

const (
	Oslo      Office   = "OSLO"
	Bengaluru Office   = "BENGALURU"
	NOK       Currency = "NOK"
	INR       Currency = "INR"

	StatusOpen       RefillStatus = "OPEN"
	StatusAssigned   RefillStatus = "ASSIGNED"
	StatusPurchased  RefillStatus = "PURCHASED"
	StatusSubmitted  RefillStatus = "SUBMITTED"
	StatusReimbursed RefillStatus = "REIMBURSED"
)

type Volunteer struct {
	ID              string
	Name            string
	HomeOffice      Office
	CurrentOffice   Office // Update this when temporarily visiting another office.
	PayrollCurrency Currency
	Available       bool
	AssignmentCount int
	LastAssignedAt  time.Time
}

type Refill struct {
	ID                string
	Office            Office
	Snack             string
	Status            RefillStatus
	AssignedVolunteer string
	CreatedAt         time.Time
	AssignedAt        time.Time
	Deadline          time.Time
	TriedVolunteerIDs []string
	PurchaseID        string
}

type Purchase struct {
	ID                    string
	RefillID              string
	Office                Office
	VolunteerID           string
	Snack                 string
	Quantity              int
	Amount                float64
	Currency              Currency // Currency in which the office purchase was made.
	PurchasedAt           time.Time
	ReceiptURL            string
	Description           string
	ReimbursementID       string
	ReimbursementAmount   float64
	ReimbursementCurrency Currency // Employee's payroll currency.
}

type OfficeBudget struct {
	Office   Office
	Currency Currency
	Monthly  float64
}

type MonthlyReport struct {
	Month      string
	ByOffice   map[Office]float64
	ByCurrency map[Currency]float64
	Budgets    map[Office]float64
	Remaining  map[Office]float64
}
