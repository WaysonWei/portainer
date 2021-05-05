package backup

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	httperror "github.com/portainer/libhttp/error"
	portainer "github.com/portainer/portainer/api"
	"github.com/portainer/portainer/api/adminmonitor"
	"github.com/portainer/portainer/api/http/offlinegate"
	"github.com/portainer/portainer/api/http/security"
)

// Handler is an http handler responsible for backup and restore portainer state
type Handler struct {
	*mux.Router
	demoEnvironment bool
	bouncer         *security.RequestBouncer
	dataStore       portainer.DataStore
	gate            *offlinegate.OfflineGate
	filestorePath   string
	shutdownTrigger context.CancelFunc
	adminMonitor    *adminmonitor.Monitor
}

// NewHandler creates an new instance of backup handler
func NewHandler(bouncer *security.RequestBouncer, dataStore portainer.DataStore, gate *offlinegate.OfflineGate, filestorePath string, shutdownTrigger context.CancelFunc, adminMonitor *adminmonitor.Monitor, demo bool) *Handler {
	h := &Handler{
		Router:          mux.NewRouter(),
		demoEnvironment: demo,
		bouncer:         bouncer,
		dataStore:       dataStore,
		gate:            gate,
		filestorePath:   filestorePath,
		shutdownTrigger: shutdownTrigger,
		adminMonitor:    adminMonitor,
	}

	h.Handle("/backup", h.restrictDemoEnv(bouncer.RestrictedAccess(adminAccess(httperror.LoggerHandler(h.backup))))).Methods(http.MethodPost)
	h.Handle("/restore", h.restrictDemoEnv(bouncer.PublicAccess(httperror.LoggerHandler(h.restore)))).Methods(http.MethodPost)

	return h
}

func adminAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityContext, err := security.RetrieveRestrictedRequestContext(r)
		if err != nil {
			httperror.WriteError(w, http.StatusInternalServerError, "Unable to retrieve user info from request context", err)
		}

		if !securityContext.IsAdmin {
			httperror.WriteError(w, http.StatusUnauthorized, "User is not authorized to perform the action", nil)
		}

		next.ServeHTTP(w, r)
	})
}

// restrict backup functionality on demo environments
func (handler *Handler) restrictDemoEnv(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handler.demoEnvironment {
			httperror.WriteError(w, http.StatusBadRequest, "This feature is not available in the demo version of Portainer", errors.New("this feature is not available in the demo version of Portainer"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func systemWasInitialized(dataStore portainer.DataStore) (bool, error) {
	users, err := dataStore.User().UsersByRole(portainer.AdministratorRole)
	if err != nil {
		return false, err
	}
	return len(users) > 0, nil
}