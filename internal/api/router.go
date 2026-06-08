package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/flowctl/flowctl/internal/api/handler"
	"github.com/flowctl/flowctl/internal/api/middleware"
	"github.com/flowctl/flowctl/internal/api/websocket"
	"github.com/flowctl/flowctl/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	Pool         *pgxpool.Pool
	AuthService  *auth.Service
	RBACEngine   *auth.RBACEngine
	OIDCProvider *auth.OIDCProvider
}

func NewRouter(deps *Dependencies) chi.Router {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Tenant-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	authMW := middleware.NewAuthMiddleware(deps.AuthService)
	tenantMW := middleware.NewTenantMiddleware(deps.Pool)
	rbacMW := middleware.NewRBACMiddleware(deps.RBACEngine)

	authHandler := handler.NewAuthHandler(deps.AuthService, deps.OIDCProvider, deps.Pool)
	workflowHandler := handler.NewWorkflowHandler(deps.Pool)
	executionHandler := handler.NewExecutionHandler(deps.Pool)
	approvalHandler := handler.NewApprovalHandler(deps.Pool)
	tenantHandler := handler.NewTenantHandler(deps.Pool)
	wsHandler := websocket.NewHandler(deps.Pool)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// Public auth routes
	r.Group(func(r chi.Router) {
		r.Post("/auth/login", authHandler.Login)
		r.Get("/auth/callback", authHandler.OIDCCallback)
		r.Post("/auth/refresh", authHandler.Refresh)
		r.Post("/auth/saml/acs", authHandler.SAMLACS)
	})

	// Authenticated API routes
	r.Group(func(r chi.Router) {
		r.Use(authMW.Authenticate)
		r.Use(tenantMW.InjectTenant)

		// Workflows
		r.Route("/api/v1/workflows", func(r chi.Router) {
			r.With(rbacMW.Require("workflow:*", "read")).Get("/", workflowHandler.List)
			r.With(rbacMW.Require("workflow:*", "create")).Post("/", workflowHandler.Create)
			r.Route("/{workflowID}", func(r chi.Router) {
				r.With(rbacMW.Require("workflow:*", "read")).Get("/", workflowHandler.Get)
				r.With(rbacMW.Require("workflow:*", "update")).Put("/", workflowHandler.Update)
				r.With(rbacMW.Require("workflow:*", "delete")).Delete("/", workflowHandler.Delete)
				r.With(rbacMW.Require("workflow:*", "publish")).Post("/versions", workflowHandler.Publish)
				r.With(rbacMW.Require("workflow:*", "publish")).Post("/rollback", workflowHandler.Rollback)
				r.Get("/versions", workflowHandler.ListVersions)
				r.With(rbacMW.Require("execution:*", "create")).Post("/run", executionHandler.Start)
			})
		})

		// Executions
		r.Route("/api/v1/executions", func(r chi.Router) {
			r.With(rbacMW.Require("execution:*", "read")).Get("/", executionHandler.List)
			r.Route("/{executionID}", func(r chi.Router) {
				r.With(rbacMW.Require("execution:*", "read")).Get("/", executionHandler.Get)
				r.With(rbacMW.Require("execution:*", "cancel")).Post("/cancel", executionHandler.Cancel)
				r.With(rbacMW.Require("execution:*", "create")).Post("/retry", executionHandler.Retry)
				r.Get("/steps", executionHandler.ListSteps)
				r.Get("/logs", executionHandler.GetLogs)
			})
		})

		// Approvals
		r.Route("/api/v1/approvals", func(r chi.Router) {
			r.With(rbacMW.Require("approval:*", "read")).Get("/", approvalHandler.List)
			r.With(rbacMW.Require("approval:*", "approve")).Post("/{approvalID}/approve", approvalHandler.Approve)
			r.With(rbacMW.Require("approval:*", "approve")).Post("/{approvalID}/reject", approvalHandler.Reject)
		})

		// Tenants
		r.Route("/api/v1/tenants", func(r chi.Router) {
			r.Get("/", tenantHandler.List)
			r.With(rbacMW.Require("tenant:*", "create")).Post("/", tenantHandler.Create)
			r.Route("/{tenantID}", func(r chi.Router) {
				r.Get("/", tenantHandler.Get)
				r.With(rbacMW.Require("tenant:*", "update")).Put("/", tenantHandler.Update)
				r.Get("/members", tenantHandler.ListMembers)
				r.With(rbacMW.Require("tenant:*", "admin")).Post("/members", tenantHandler.AddMember)
			})
		})

		// Secrets
		r.Route("/api/v1/secrets", func(r chi.Router) {
			r.With(rbacMW.Require("secret:*", "read")).Get("/", handler.ListSecrets(deps.Pool))
			r.With(rbacMW.Require("secret:*", "create")).Post("/", handler.CreateSecret(deps.Pool))
			r.With(rbacMW.Require("secret:*", "delete")).Delete("/{secretID}", handler.DeleteSecret(deps.Pool))
		})

		// Cron schedules
		r.Route("/api/v1/cron", func(r chi.Router) {
			r.With(rbacMW.Require("cron:*", "read")).Get("/", handler.ListCronSchedules(deps.Pool))
			r.With(rbacMW.Require("cron:*", "create")).Post("/", handler.CreateCronSchedule(deps.Pool))
			r.With(rbacMW.Require("cron:*", "update")).Put("/{cronID}", handler.UpdateCronSchedule(deps.Pool))
			r.With(rbacMW.Require("cron:*", "delete")).Delete("/{cronID}", handler.DeleteCronSchedule(deps.Pool))
		})

		// Audit logs
		r.With(rbacMW.Require("audit:*", "read")).Get("/api/v1/audit", handler.ListAuditLogs(deps.Pool))

		// Users/Roles
		r.Route("/api/v1/users", func(r chi.Router) {
			r.Get("/me", authHandler.Me)
			r.With(rbacMW.Require("user:*", "read")).Get("/", handler.ListUsers(deps.Pool))
		})
		r.Route("/api/v1/roles", func(r chi.Router) {
			r.With(rbacMW.Require("role:*", "read")).Get("/", handler.ListRoles(deps.Pool))
			r.With(rbacMW.Require("role:*", "create")).Post("/", handler.CreateRole(deps.Pool))
		})
	})

	// WebSocket (auth handled inside)
	r.Get("/ws/logs/{executionID}", wsHandler.StreamLogs)

	return r
}
