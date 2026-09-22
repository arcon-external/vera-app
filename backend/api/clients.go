package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// clientDTO renders one client row with its archive size.
func clientDTO(id int64, code int16, name, address string, quotationCount int64) map[string]any {
	return map[string]any{
		"id":             id,
		"code":           code,
		"name":           name,
		"address":        address,
		"quotationCount": quotationCount,
	}
}

func parseClientID(context echo.Context) (int64, bool) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		_ = context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid client id"})
		return 0, false
	}
	return id, true
}

// listClients serves the client directory to every signed-in role: the
// editor dropdown reads here.
func (s *Server) listClients(context echo.Context) error {
	clients, err := s.database.ListClients(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "list clients"})
	}
	result := make([]map[string]any, 0, len(clients))
	for _, client := range clients {
		result = append(result, clientDTO(client.ID, client.Code, client.Name, client.PrimaryAddress, client.QuotationCount))
	}
	return context.JSON(http.StatusOK, map[string]any{"clients": result})
}

func (s *Server) createClient(context echo.Context) error {
	var request struct {
		Code    int    `json:"code"`
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	client, err := s.database.CreateClient(context.Request().Context(), request.Code, request.Name, request.Address)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "already in use"):
			return context.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
		case strings.Contains(err.Error(), "required"),
			strings.Contains(err.Error(), "invalid client code"),
			strings.Contains(err.Error(), "exhausted"):
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		default:
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "create client"})
		}
	}
	me := sessionUser(context)
	actorID := me.ID
	_ = s.database.Audit(context.Request().Context(), &actorID, "client.create", nil, client.Name)
	return context.JSON(http.StatusCreated, clientDTO(client.ID, client.Code, client.Name, client.PrimaryAddress, client.QuotationCount))
}

func (s *Server) updateClient(context echo.Context) error {
	id, ok := parseClientID(context)
	if !ok {
		return nil
	}
	var request struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		Code    *int   `json:"code"`
	}
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if request.Code != nil {
		existing, err := s.database.GetClientByID(context.Request().Context(), id)
		if err != nil {
			return context.JSON(http.StatusNotFound, map[string]string{"error": "client not found"})
		}
		if int(existing.Code) != *request.Code {
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "client code cannot be changed"})
		}
	}
	client, err := s.database.UpdateClient(context.Request().Context(), id, request.Name, request.Address)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "not found"):
			return context.JSON(http.StatusNotFound, map[string]string{"error": "client not found"})
		case strings.Contains(err.Error(), "already in use"):
			return context.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
		case strings.Contains(err.Error(), "required"):
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		default:
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "update client"})
		}
	}
	me := sessionUser(context)
	actorID := me.ID
	_ = s.database.Audit(context.Request().Context(), &actorID, "client.update", nil, client.Name)
	return context.JSON(http.StatusOK, clientDTO(client.ID, client.Code, client.Name, client.PrimaryAddress, client.QuotationCount))
}

func (s *Server) deleteClient(context echo.Context) error {
	id, ok := parseClientID(context)
	if !ok {
		return nil
	}
	name, err := s.database.DeleteClient(context.Request().Context(), id)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "not found"):
			return context.JSON(http.StatusNotFound, map[string]string{"error": "client not found"})
		case strings.Contains(err.Error(), "quotations reference it"):
			return context.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
		default:
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "delete client"})
		}
	}
	me := sessionUser(context)
	actorID := me.ID
	_ = s.database.Audit(context.Request().Context(), &actorID, "client.delete", nil, name)
	return context.JSON(http.StatusOK, map[string]string{"name": name})
}
