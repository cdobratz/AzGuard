package cost

import (
	"time"

	"github.com/agent/agent/internal/storage"
)

type CostSummary struct {
	Period          string            `json:"period"`
	TotalCost       float64           `json:"total_cost"`
	Currency        string            `json:"currency"`
	ByService       map[string]float64 `json:"by_service"`
	ByResourceGroup map[string]float64 `json:"by_resource_group"`
	Forecast        *Forecast         `json:"forecast,omitempty"`
	MonthlyBreakdown []storage.MonthlyCost `json:"monthly_breakdown,omitempty"`
	Trend           *TrendAnalysis    `json:"trend,omitempty"`
}

type Forecast struct {
	NextMonth   float64 `json:"next_month"`
	Confidence  string  `json:"confidence"`
}

type CostFilter struct {
	StartDate   string
	EndDate     string
	ServiceName string
	GroupBy     string
}

type Alert struct {
	ID              int64   `json:"id,omitempty"`
	Name            string  `json:"name"`
	Threshold       float64 `json:"threshold"`
	SubscriptionID  string  `json:"subscription_id"`
	Enabled         bool    `json:"enabled"`
}

func GetCurrentBillingPeriod() (startDate, endDate string) {
	now := time.Now()
	startDate = now.Format("2006-01-02")
	endDate = now.AddDate(0, 1, 0).Format("2006-01-02")
	return
}

func GetLastNMonths(n int) (startDate, endDate string) {
	now := time.Now()
	endDate = now.Format("2006-01-02")
	startDate = now.AddDate(0, -n, 0).Format("2006-01-02")
	return
}

func GetCurrentMonthDateRange() (startDate, endDate string) {
	now := time.Now()
	startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	if now.Month() == time.December {
		endDate = time.Date(now.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	} else {
		endDate = time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	return
}
