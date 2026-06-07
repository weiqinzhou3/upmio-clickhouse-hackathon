package platform

import (
	"context"

	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/model"
)

type Store interface {
	CreateCluster(context.Context, model.CreateClusterRequest) (model.ClusterSummary, error)
	ListClusters(context.Context, string) ([]model.ClusterSummary, error)
	GetCluster(context.Context, string, string) (model.ClusterSummary, error)
	GetClusterResources(context.Context, string, string) (model.ClusterResources, error)
	RunHealthcheck(context.Context, string, string) (model.HealthcheckReport, error)
	GetMetricsSummary(context.Context, string, string) (model.MetricsSummary, error)
	RunDiagnostics(context.Context, string, string, model.DiagnosticsFilter) (model.DiagnosticsReport, error)
}
