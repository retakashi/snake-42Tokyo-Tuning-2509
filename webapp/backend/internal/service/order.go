package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type OrderService struct {
	store *repository.Store
}

func NewOrderService(store *repository.Store) *OrderService {
	return &OrderService{store: store}
}

// ユーザーの注文履歴を取得
func (s *OrderService) FetchOrders(ctx context.Context, userID int, req model.ListRequest) ([]model.Order, int, error) {
	// トレーシングの開始
	tracer := otel.Tracer("service.order")
	ctx, span := tracer.Start(ctx, "OrderService.FetchOrders")
	defer span.End()
	span.SetAttributes(
		attribute.Int("user.id", userID),
		attribute.Int("request.page", req.Page),
		attribute.Int("request.page_size", req.PageSize),
		attribute.String("request.sort_field", req.SortField),
		attribute.String("request.sort_order", req.SortOrder),
	)
	////
	var orders []model.Order
	var total int
	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		var fetchErr error
		orders, total, fetchErr = s.store.OrderRepo.ListOrders(ctx, userID, req)
		if fetchErr != nil {

			// Record the error in the span
			span.RecordError(fetchErr)
			return fetchErr
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	// Add response details to span
	span.SetAttributes(
		attribute.Int("response.total", total),
		attribute.Int("response.count", len(orders)),
	)

	return orders, total, nil
}
