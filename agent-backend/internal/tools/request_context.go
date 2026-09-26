package tools

import "context"

type requestContextKey struct{}

// WithDashboardQueryContext adds authenticated, request-scoped Grafana context.
func WithDashboardQueryContext(ctx context.Context, scope DashboardQueryContext) context.Context {
	return context.WithValue(ctx, requestContextKey{}, scope)
}

// MergeDashboardQueryContext updates visible dashboard fields while preserving authenticated headers.
func MergeDashboardQueryContext(ctx context.Context, scope DashboardQueryContext) context.Context {
	current := dashboardQueryContextFrom(ctx)
	if scope.Cookie == "" {
		scope.Cookie = current.Cookie
	}
	if scope.Authorization == "" {
		scope.Authorization = current.Authorization
	}
	if scope.DashboardUID == "" {
		scope.DashboardUID = current.DashboardUID
	}
	if scope.TimeFrom == "" {
		scope.TimeFrom = current.TimeFrom
	}
	if scope.TimeTo == "" {
		scope.TimeTo = current.TimeTo
	}
	if scope.ActiveFilters == nil {
		scope.ActiveFilters = current.ActiveFilters
	}
	return WithDashboardQueryContext(ctx, scope)
}

func dashboardQueryContextFrom(ctx context.Context) DashboardQueryContext {
	scope, _ := ctx.Value(requestContextKey{}).(DashboardQueryContext)
	return scope
}
