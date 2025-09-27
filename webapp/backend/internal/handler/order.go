package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/service"
	"encoding/json"
	"log"
	"net/http"
)

type OrderHandler struct {
	OrderSvc *service.OrderService
}

func NewOrderHandler(svc *service.OrderService) *OrderHandler {
	return &OrderHandler{OrderSvc: svc}
}

// 注文履歴一覧を取得
func (h *OrderHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "User not found", http.StatusInternalServerError)
		return
	}

	var req model.ListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// デフォルト値の設定
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	allowedSortFields := map[string]string{
		"order_id":       "o.order_id",
		"product_name":   "p.name",
		"created_at":     "o.created_at",
		"shipped_status": "o.shipped_status",
		"arrived_at":     "o.arrived_at",
	}
	sanitizeListRequest(&req, allowedSortFields, "o.order_id", "desc")
	if req.Type != "" && req.Type != "partial" && req.Type != "prefix" {
		req.Type = "partial"
	}
	if req.Type == "" {
		req.Type = "partial"
	}

	orders, total, err := h.OrderSvc.FetchOrders(r.Context(), userID, req)
	if err != nil {
		log.Printf("Failed to fetch orders for user %d: %v", userID, err)
		http.Error(w, "Failed to fetch orders", http.StatusInternalServerError)
		return
	}

	resp := struct {
		Data  []model.Order `json:"data"`
		Total int           `json:"total"`
	}{
		Data:  orders,
		Total: total,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
