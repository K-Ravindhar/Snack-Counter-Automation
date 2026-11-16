package main

import (
	"fmt"
	"time"

	"snack-counter-automation/internal/integrations"
	"snack-counter-automation/internal/models"
	"snack-counter-automation/internal/service"
)

func main() {
	chat, hr := &integrations.MockChat{}, &integrations.MockHR{}
	svc := service.New(chat, hr, integrations.SameCurrencyOnly{})
	now := time.Now()
	_ = svc.AddVolunteer(models.Volunteer{ID: "emp-001", Name: "Asha", HomeOffice: models.Bengaluru, CurrentOffice: models.Bengaluru, PayrollCurrency: models.INR, Available: true})
	refill, err := svc.ReportEmpty(models.Bengaluru, "Coffee", now)
	if err != nil {
		panic(err)
	}
	purchase, err := svc.CompletePurchase(refill.ID, "emp-001", 1200, 4, "https://receipt.example/123", now)
	if err != nil {
		panic(err)
	}
	claim, err := svc.SubmitReimbursement(purchase.ID)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Refill %s status: SUBMITTED; HR claim: %s\n", refill.ID, claim.ReimbursementID)
	report := svc.MonthlyReport(now.Year(), now.Month())
	fmt.Printf("Monthly spending: Oslo %.2f NOK; Bengaluru %.2f INR\n", report.ByOffice[models.Oslo], report.ByOffice[models.Bengaluru])
}
