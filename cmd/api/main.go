package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agent/agent/internal/cloud/aws"
	"github.com/agent/agent/internal/cloud/azure"
	"github.com/agent/agent/internal/cloud/gcp"
	"github.com/agent/agent/internal/config"
	"github.com/agent/agent/internal/cost"
	"github.com/agent/agent/internal/storage"
)

type APIServer struct {
	db        *storage.DB
	costSvc   *cost.Service
	awsClient *aws.CostClient
	gcpClient *gcp.CostClient
	config    *config.Config
	server    *http.Server
}

func NewAPIServer(cfg *config.Config, db *storage.DB) *APIServer {
	s := &APIServer{
		db:      db,
		config:  cfg,
	}

	tokenProvider, _ := azure.NewTokenProvider(cfg.Azure.AuthMethod, map[string]string{
		"tenant_id":     cfg.Azure.TenantID,
		"client_id":     cfg.Azure.ClientID,
		"client_secret": cfg.Azure.ClientSecret,
	})

	azureCostClient := azure.NewCostClient(cfg.Azure.SubscriptionID, tokenProvider)
	s.costSvc = cost.NewService(db, azureCostClient)

	if cfg.AWS.AccessKey != "" {
		s.awsClient = aws.NewCostClient(cfg.AWS.AccessKey, cfg.AWS.SecretKey, cfg.AWS.SessionToken, cfg.AWS.Region)
	}

	if cfg.GCP.ProjectID != "" {
		s.gcpClient = gcp.NewCostClient(cfg.GCP.ProjectID)
	}

	return s
}

func (s *APIServer) SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.handleHealth)

	mux.HandleFunc("/api/v1/cost/azure/current", s.handleAzureCurrentCost)
	mux.HandleFunc("/api/v1/cost/azure/summary", s.handleAzureSummary)
	mux.HandleFunc("/api/v1/cost/azure/history", s.handleAzureHistory)
	mux.HandleFunc("/api/v1/cost/azure/forecast", s.handleAzureForecast)
	mux.HandleFunc("/api/v1/cost/azure/trend", s.handleAzureTrend)

	mux.HandleFunc("/api/v1/cost/aws/current", s.handleAWSCurrentCost)
	mux.HandleFunc("/api/v1/cost/aws/forecast", s.handleAWSForecast)

	mux.HandleFunc("/api/v1/cost/gcp/current", s.handleGCPCurrentCost)
	mux.HandleFunc("/api/v1/cost/gcp/forecast", s.handleGCPForecast)

	mux.HandleFunc("/api/v1/cost/all", s.handleAllCosts)
	mux.HandleFunc("/api/v1/cost/report", s.handleReport)

	mux.HandleFunc("/api/v1/alerts", s.handleAlerts)
	mux.HandleFunc("/api/v1/alerts/check", s.handleAlertsCheck)

	mux.HandleFunc("/api/v1/config", s.handleConfig)

	return mux
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "timestamp": time.Now().Format(time.RFC3339)})
}

func (s *APIServer) handleAzureCurrentCost(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	summary, err := s.costSvc.GetCurrentCosts(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(summary)
}

func (s *APIServer) handleAzureSummary(w http.ResponseWriter, r *http.Request) {
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	summary, err := s.costSvc.GetCostSummary(cost.CostFilter{
		StartDate: startDate,
		EndDate:   endDate,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(summary)
}

func (s *APIServer) handleAzureHistory(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		fmt.Sscanf(d, "%d", &days)
	}

	summary, err := s.costSvc.GetCostHistory(days)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(summary)
}

func (s *APIServer) handleAzureForecast(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	forecast, err := s.costSvc.GetForecast(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(forecast)
}

func (s *APIServer) handleAzureTrend(w http.ResponseWriter, r *http.Request) {
	trend, err := s.costSvc.GetTrendAnalysis()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(trend)
}

func (s *APIServer) handleAWSCurrentCost(w http.ResponseWriter, r *http.Request) {
	if s.awsClient == nil {
		http.Error(w, "AWS not configured", 400)
		return
	}

	ctx := context.Background()
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	if startDate == "" || endDate == "" {
		startDate, endDate = cost.GetCurrentMonthDateRange()
	}

	result, err := s.awsClient.QueryCosts(ctx, startDate, endDate)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(result)
}

func (s *APIServer) handleAWSForecast(w http.ResponseWriter, r *http.Request) {
	if s.awsClient == nil {
		http.Error(w, "AWS not configured", 400)
		return
	}

	ctx := context.Background()
	result, err := s.awsClient.GetForecast(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(result)
}

func (s *APIServer) handleGCPCurrentCost(w http.ResponseWriter, r *http.Request) {
	if s.gcpClient == nil {
		http.Error(w, "GCP not configured", 400)
		return
	}

	ctx := context.Background()
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	if startDate == "" || endDate == "" {
		startDate, endDate = cost.GetCurrentMonthDateRange()
	}

	result, err := s.gcpClient.QueryCosts(ctx, startDate, endDate)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(result)
}

func (s *APIServer) handleGCPForecast(w http.ResponseWriter, r *http.Request) {
	if s.gcpClient == nil {
		http.Error(w, "GCP not configured", 400)
		return
	}

	ctx := context.Background()
	result, err := s.gcpClient.GetForecast(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(result)
}

func (s *APIServer) handleAllCosts(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"providers": map[string]interface{}{},
	}

	if s.config.Azure.SubscriptionID != "" {
		summary, _ := s.costSvc.GetCostSummary(cost.CostFilter{})
		response["providers"].(map[string]interface{})["azure"] = summary
	}

	if s.config.AWS.Region != "" {
		response["providers"].(map[string]interface{})["aws"] = map[string]string{"status": "not_implemented"}
	}

	if s.config.GCP.ProjectID != "" {
		response["providers"].(map[string]interface{})["gcp"] = map[string]string{"status": "not_implemented"}
	}

	json.NewEncoder(w).Encode(response)
}

func (s *APIServer) handleReport(w http.ResponseWriter, r *http.Request) {
	report, err := s.costSvc.GenerateReport()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(report)
}

func (s *APIServer) handleAlerts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		alerts, err := s.db.GetAlerts()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(alerts)

	case "POST":
		var alert struct {
			Name      string  `json:"name"`
			Threshold float64 `json:"threshold"`
		}
		if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		err := s.db.SaveAlert(storage.Alert{
			Name:      alert.Name,
			Threshold: alert.Threshold,
			Enabled:   true,
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "created"})

	case "DELETE":
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "name required", 400)
			return
		}
		err := s.db.DeleteAlert(name)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}
}

func (s *APIServer) handleAlertsCheck(w http.ResponseWriter, r *http.Request) {
	summary, err := s.costSvc.GetCostSummary(cost.CostFilter{})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	alerts, err := s.db.GetAlerts()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var triggered []string
	for _, a := range alerts {
		if a.Enabled && summary.TotalCost >= a.Threshold {
			triggered = append(triggered, a.Name)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"current_cost": summary.TotalCost,
		"alerts":       alerts,
		"triggered":    triggered,
	})
}

func (s *APIServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"azure": map[string]string{
			"subscription_id": s.config.Azure.SubscriptionID,
			"auth_method":     s.config.Azure.AuthMethod,
		},
		"aws": map[string]string{
			"region": s.config.AWS.Region,
		},
		"gcp": map[string]string{
			"project_id": s.config.GCP.ProjectID,
		},
		"ollama": map[string]string{
			"base_url": s.config.Ollama.BaseURL,
			"model":    s.config.Ollama.Model,
		},
	})
}

func (s *APIServer) Start(port string) error {
	addr := ":" + port
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.SetupRoutes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("Starting API server on %s", addr)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	return nil
}

func (s *APIServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

var (
	port = flag.String("port", "8080", "API server port")
)

func main() {
	flag.Parse()

	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db, err := storage.New(cfg.Storage.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	server := NewAPIServer(cfg, db)

	if err := server.Start(*port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	server.Stop()
	log.Println("Server stopped")
}
