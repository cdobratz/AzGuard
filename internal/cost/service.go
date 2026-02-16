package cost

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/azguard/azguard/internal/cloud/azure"
	"github.com/azguard/azguard/internal/storage"
)

type Service struct {
	db        *storage.DB
	azureCost *azure.CostClient
}

func NewService(db *storage.DB, azureCost *azure.CostClient) *Service {
	return &Service{
		db:        db,
		azureCost: azureCost,
	}
}

func (s *Service) FetchAndStoreCosts(ctx context.Context, startDate, endDate string) error {
	result, err := s.azureCost.QueryCostsByService(ctx, startDate, endDate)
	if err != nil {
		return fmt.Errorf("failed to query costs: %w", err)
	}

	records := make([]storage.CostRecord, len(result.Records))
	for i, r := range result.Records {
		records[i] = storage.CostRecord{
			SubscriptionID: s.azureCost.SubscriptionID,
			ResourceGroup:  r.ResourceGroup,
			ServiceName:    r.ServiceName,
			Cost:           r.Cost,
			Currency:       r.Currency,
			Date:           r.Date,
		}
	}

	if err := s.db.SaveCostRecords(records); err != nil {
		return fmt.Errorf("failed to save cost records: %w", err)
	}

	return nil
}

func (s *Service) GetCostSummary(filter CostFilter) (*CostSummary, error) {
	byService, err := s.db.GetAggregatedCosts(storage.CostFilter{
		StartDate: filter.StartDate,
		EndDate:   filter.EndDate,
		GroupBy:   "ServiceName",
	})
	if err != nil {
		return nil, err
	}

	byResourceGroup, err := s.db.GetAggregatedCosts(storage.CostFilter{
		StartDate: filter.StartDate,
		EndDate:   filter.EndDate,
		GroupBy:   "ResourceGroup",
	})
	if err != nil {
		return nil, err
	}

	var totalCost float64
	for _, c := range byService {
		totalCost += c
	}

	summary := &CostSummary{
		Period:           filter.StartDate + " to " + filter.EndDate,
		TotalCost:        totalCost,
		Currency:         "USD",
		ByService:        byService,
		ByResourceGroup: byResourceGroup,
	}

	return summary, nil
}

func (s *Service) GetForecast(ctx context.Context) (*Forecast, error) {
	localForecast, err := s.GetLocalForecast()
	if err == nil && localForecast.Confidence != "low" {
		return localForecast, nil
	}

	result, err := s.azureCost.GetForecast(ctx, "Monthly")
	if err != nil {
		if localForecast != nil {
			return localForecast, nil
		}
		return nil, fmt.Errorf("both local and API forecast failed: %w", err)
	}

	return &Forecast{
		NextMonth:  result.TotalCost,
		Confidence: "medium",
	}, nil
}

func (s *Service) GetCurrentCosts(ctx context.Context) (*CostSummary, error) {
	startDate, endDate := GetCurrentMonthDateRange()

	if err := s.FetchAndStoreCosts(ctx, startDate, endDate); err != nil {
		return nil, err
	}

	summary, err := s.GetCostSummary(CostFilter{
		StartDate: startDate,
		EndDate:   endDate,
	})
	if err != nil {
		return nil, err
	}

	forecast, err := s.GetForecast(ctx)
	if err == nil {
		summary.Forecast = forecast
	}

	return summary, nil
}

func (s *Service) GetCostHistory(days int) (*CostSummary, error) {
	startDate, endDate := GetLastNMonths(days)

	summary, err := s.GetCostSummary(CostFilter{
		StartDate: startDate,
		EndDate:   endDate,
	})
	if err != nil {
		return nil, err
	}

	monthlyCosts, err := s.db.GetMonthlyCosts(12)
	if err == nil && len(monthlyCosts) > 0 {
		summary.MonthlyBreakdown = monthlyCosts
	}

	return summary, nil
}

type TrendAnalysis struct {
	CurrentMonth    float64           `json:"current_month"`
	PreviousMonth  float64           `json:"previous_month"`
	ChangePercent  float64           `json:"change_percent"`
	Trend          string            `json:"trend"`
	AverageMonthly float64           `json:"average_monthly"`
	Projection     float64           `json:"projection"`
}

func (s *Service) GetTrendAnalysis() (*TrendAnalysis, error) {
	monthlyCosts, err := s.db.GetMonthlyCosts(6)
	if err != nil {
		return nil, fmt.Errorf("failed to get monthly costs: %w", err)
	}

	if len(monthlyCosts) == 0 {
		return &TrendAnalysis{
			CurrentMonth:   0,
			PreviousMonth: 0,
			ChangePercent: 0,
			Trend:         "no_data",
			AverageMonthly: 0,
			Projection:    0,
		}, nil
	}

	currentMonth := monthlyCosts[0].TotalCost
	var previousMonth float64
	if len(monthlyCosts) > 1 {
		previousMonth = monthlyCosts[1].TotalCost
	}

	var changePercent float64
	if previousMonth > 0 {
		changePercent = ((currentMonth - previousMonth) / previousMonth) * 100
	}

	trend := "stable"
	if changePercent > 5 {
		trend = "increasing"
	} else if changePercent < -5 {
		trend = "decreasing"
	}

	var sum float64
	for _, m := range monthlyCosts {
		sum += m.TotalCost
	}
	averageMonthly := sum / float64(len(monthlyCosts))

	projection := s.calculateProjection(monthlyCosts)

	return &TrendAnalysis{
		CurrentMonth:   currentMonth,
		PreviousMonth:  previousMonth,
		ChangePercent:  math.Round(changePercent*100) / 100,
		Trend:          trend,
		AverageMonthly: math.Round(averageMonthly*100) / 100,
		Projection:     math.Round(projection*100) / 100,
	}, nil
}

func (s *Service) calculateProjection(monthlyCosts []storage.MonthlyCost) float64 {
	if len(monthlyCosts) < 2 {
		return 0
	}

	n := float64(len(monthlyCosts))
	var sumX, sumY, sumXY, sumX2 float64

	for i, mc := range monthlyCosts {
		x := float64(i)
		y := mc.TotalCost
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	intercept := (sumY - slope*sumX) / n

	nextMonthIndex := float64(len(monthlyCosts))
	return slope*nextMonthIndex + intercept
}

func (s *Service) GetLocalForecast() (*Forecast, error) {
	monthlyCosts, err := s.db.GetMonthlyCosts(6)
	if err != nil {
		return nil, err
	}

	if len(monthlyCosts) < 2 {
		return &Forecast{
			NextMonth:  0,
			Confidence: "low",
		}, nil
	}

	projection := s.calculateProjection(monthlyCosts)
	if projection < 0 {
		projection = 0
	}

	confidence := "low"
	if len(monthlyCosts) >= 4 {
		confidence = "medium"
	}
	if len(monthlyCosts) >= 6 {
		confidence = "high"
	}

	return &Forecast{
		NextMonth:  math.Round(projection*100) / 100,
		Confidence: confidence,
	}, nil
}

func (s *Service) GenerateReport() (*Report, error) {
	monthlyCosts, err := s.db.GetMonthlyCosts(12)
	if err != nil {
		return nil, err
	}

	summary, err := s.GetCostSummary(CostFilter{})
	if err != nil {
		return nil, err
	}

	forecast, _ := s.GetLocalForecast()

	var monthlyData []MonthlyReport
	for _, m := range monthlyCosts {
		monthlyData = append(monthlyData, MonthlyReport{
			Month:     m.Month,
			TotalCost: m.TotalCost,
			Currency:  m.Currency,
		})
	}

	var topServices []ServiceCost
	for service, cost := range summary.ByService {
		topServices = append(topServices, ServiceCost{
			Service: service,
			Cost:    cost,
		})
	}

	period := "Last 12 months"
	if len(monthlyCosts) > 0 {
		period = monthlyCosts[len(monthlyCosts)-1].Month + " to " + monthlyCosts[0].Month
	}

	report := &Report{
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
		Period:      period,
		TotalCost:   summary.TotalCost,
		Currency:    summary.Currency,
		Forecast:    0,
		MonthlyData: monthlyData,
		TopServices: topServices,
	}

	if forecast != nil {
		report.Forecast = forecast.NextMonth
	}

	return report, nil
}
