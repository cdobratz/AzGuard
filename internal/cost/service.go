package cost

import (
	"context"
	"fmt"

	"github.com/agent/agent/internal/cloud/azure"
	"github.com/agent/agent/internal/storage"
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
	result, err := s.azureCost.GetForecast(ctx, "Monthly")
	if err != nil {
		return nil, err
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

	return summary, nil
}
