package service

import (
	"context"
	"testing"

	"backend/internal/model"
)

func TestSelectOrdersForDeliveryBasic(t *testing.T) {
	orders := []model.Order{
		{OrderID: 1, Weight: 5, Value: 10},
		{OrderID: 2, Weight: 4, Value: 40},
		{OrderID: 3, Weight: 6, Value: 30},
	}

	plan, err := selectOrdersForDelivery(context.Background(), orders, "robot", 9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.TotalWeight != 9 {
		t.Fatalf("expected total weight 9, got %d", plan.TotalWeight)
	}
	if plan.TotalValue != 50 {
		t.Fatalf("expected total value 50, got %d", plan.TotalValue)
	}

	if len(plan.Orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(plan.Orders))
	}

	if plan.Orders[0].OrderID != 1 || plan.Orders[1].OrderID != 2 {
		t.Fatalf("unexpected order selection: %+v", plan.Orders)
	}
}

func TestSelectOrdersForDeliveryNoOrders(t *testing.T) {
	plan, err := selectOrdersForDelivery(context.Background(), nil, "robot", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Orders == nil {
		t.Fatalf("expected empty slice, got nil")
	}

	plan, err = selectOrdersForDelivery(context.Background(), nil, "robot", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Orders == nil {
		t.Fatalf("expected empty slice when capacity zero")
	}
}

func TestSelectOrdersForDeliveryZeroWeight(t *testing.T) {
	orders := []model.Order{
		{OrderID: 1, Weight: 0, Value: 5},
		{OrderID: 2, Weight: 2, Value: 3},
		{OrderID: 3, Weight: 3, Value: 8},
	}

	plan, err := selectOrdersForDelivery(context.Background(), orders, "robot", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.TotalWeight != 2 {
		t.Fatalf("expected total weight 2, got %d", plan.TotalWeight)
	}
	if plan.TotalValue != 8 {
		t.Fatalf("expected total value 8, got %d", plan.TotalValue)
	}

	if len(plan.Orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(plan.Orders))
	}

	if plan.Orders[0].OrderID != 1 {
		t.Fatalf("expected zero-weight order to be included first, got %+v", plan.Orders)
	}
}

func TestSelectOrdersForDeliveryZeroValue(t *testing.T) {
	orders := []model.Order{
		{OrderID: 1, Weight: 3, Value: 0},
		{OrderID: 2, Weight: 4, Value: 0},
	}

	plan, err := selectOrdersForDelivery(context.Background(), orders, "robot", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Orders) == 0 {
		t.Fatalf("expected at least one order, got none")
	}
	if plan.Orders[0].OrderID != 1 {
		t.Fatalf("expected deterministic fallback selection, got %+v", plan.Orders)
	}
}

func TestSelectOrdersForDeliveryContextCanceled(t *testing.T) {
	orders := make([]model.Order, 5)
	for i := range orders {
		orders[i] = model.Order{OrderID: int64(i + 1), Weight: 1, Value: 1}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := selectOrdersForDelivery(ctx, orders, "robot", 3)
	if err == nil {
		t.Fatalf("expected error due to context cancellation")
	}
}
