package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"
	"context"
	"log"
	"os"
	"strconv"
)

type RobotService struct {
	store        *repository.Store
	cloneEnabled bool
	supplyTarget int
}

func NewRobotService(store *repository.Store) *RobotService {
	cloneEnabled := true
	if v := os.Getenv("ROBOT_SHIPPING_CLONE_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cloneEnabled = b
		}
	}

	supplyTarget := 500
	if v := os.Getenv("ROBOT_SHIPPING_SUPPLY_TARGET"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			supplyTarget = n
		}
	}
	if supplyTarget < 0 {
		supplyTarget = 0
	}

	return &RobotService{
		store:        store,
		cloneEnabled: cloneEnabled,
		supplyTarget: supplyTarget,
	}
}

func (s *RobotService) GenerateDeliveryPlan(ctx context.Context, robotID string, capacity int) (*model.DeliveryPlan, error) {
	var plan model.DeliveryPlan

	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.ExecTx(ctx, func(txStore *repository.Store) error {
			orders, err := txStore.OrderRepo.GetShippingOrders(ctx)
			if err != nil {
				return err
			}
			plan, err = selectOrdersForDelivery(ctx, orders, robotID, capacity)
			if err != nil {
				return err
			}
			if len(plan.Orders) > 0 {
				orderIDs := make([]int64, len(plan.Orders))
				for i, order := range plan.Orders {
					orderIDs[i] = order.OrderID
				}

				if err := txStore.OrderRepo.UpdateStatuses(ctx, orderIDs, "delivering"); err != nil {
					return err
				}
				log.Printf("Updated status to 'delivering' for %d orders", len(orderIDs))
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

func (s *RobotService) UpdateOrderStatus(ctx context.Context, orderID int64, newStatus string) error {
	return utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.ExecTx(ctx, func(txStore *repository.Store) error {
			if err := txStore.OrderRepo.UpdateStatuses(ctx, []int64{orderID}, newStatus); err != nil {
				return err
			}
			if newStatus == "completed" && s.cloneEnabled && s.supplyTarget > 0 {
				shippingCount, err := txStore.OrderRepo.CountShipping(ctx)
				if err != nil {
					return err
				}
				if shippingCount < s.supplyTarget {
					if err := txStore.OrderRepo.CloneAsShipping(ctx, []int64{orderID}); err != nil {
						return err
					}
				}
			}
			return nil
		})
	})
}

type pathNode struct {
	itemIndex int
	prevIdx   int
}

func selectOrdersForDelivery(ctx context.Context, orders []model.Order, robotID string, robotCapacity int) (model.DeliveryPlan, error) {
	if robotCapacity <= 0 || len(orders) == 0 {
		return model.DeliveryPlan{RobotID: robotID, Orders: make([]model.Order, 0)}, nil
	}

	// フィルタ: 積載量を超える注文は候補外に
	filtered := orders[:0]
	for _, o := range orders {
		if o.Weight <= robotCapacity {
			filtered = append(filtered, o)
		}
	}
	orders = filtered
	if len(orders) == 0 {
		return model.DeliveryPlan{RobotID: robotID, Orders: make([]model.Order, 0)}, nil
	}

	var zeroWeightOrders []model.Order
	positiveOrders := orders[:0]
	totalWeight := 0
	for _, o := range orders {
		if o.Weight == 0 {
			zeroWeightOrders = append(zeroWeightOrders, o)
			continue
		}
		positiveOrders = append(positiveOrders, o)
		totalWeight += o.Weight
	}

	selected := make([]model.Order, 0, len(orders))
	selected = append(selected, zeroWeightOrders...)
	totalValue := 0
	for _, o := range zeroWeightOrders {
		totalValue += o.Value
	}

	if len(positiveOrders) == 0 {
		return model.DeliveryPlan{
			RobotID:     robotID,
			TotalWeight: 0,
			TotalValue:  totalValue,
			Orders:      selected,
		}, nil
	}

	effectiveCap := robotCapacity
	if totalWeight < effectiveCap {
		effectiveCap = totalWeight
	}
	if effectiveCap <= 0 {
		return model.DeliveryPlan{
			RobotID:     robotID,
			TotalWeight: 0,
			TotalValue:  totalValue,
			Orders:      selected,
		}, nil
	}

	bestValue := make([]int, effectiveCap+1)
	bestPathIdx := make([]int, effectiveCap+1)
	for i := range bestPathIdx {
		bestPathIdx[i] = -1
	}
	paths := make([]pathNode, 0, len(positiveOrders))

	const checkEvery = 4096
	steps := 0

	for i, order := range positiveOrders {
		if err := ctx.Err(); err != nil {
			return model.DeliveryPlan{}, err
		}
		w := order.Weight
		if w > effectiveCap {
			continue
		}
		v := order.Value
		for currentCap := effectiveCap; currentCap >= w; currentCap-- {
			candidate := bestValue[currentCap-w] + v
			if candidate > bestValue[currentCap] {
				bestValue[currentCap] = candidate
				prevIdx := bestPathIdx[currentCap-w]
				pathIdx := len(paths)
				paths = append(paths, pathNode{itemIndex: i, prevIdx: prevIdx})
				bestPathIdx[currentCap] = pathIdx
			}
			steps++
			if steps%checkEvery == 0 {
				select {
				case <-ctx.Done():
					return model.DeliveryPlan{}, ctx.Err()
				default:
				}
			}
		}
	}

	bestCap := 0
	maxValue := 0
	for cap := 0; cap <= effectiveCap; cap++ {
		if bestValue[cap] > maxValue {
			maxValue = bestValue[cap]
			bestCap = cap
		}
	}

	for idx := bestPathIdx[bestCap]; idx != -1; idx = paths[idx].prevIdx {
		selected = append(selected, positiveOrders[paths[idx].itemIndex])
	}

	if len(selected) == len(zeroWeightOrders) {
		fallbackIdx := -1
		for i, order := range positiveOrders {
			if order.Weight > effectiveCap {
				continue
			}
			if fallbackIdx == -1 || order.Value > positiveOrders[fallbackIdx].Value || (order.Value == positiveOrders[fallbackIdx].Value && order.Weight < positiveOrders[fallbackIdx].Weight) {
				fallbackIdx = i
			}
		}
		if fallbackIdx != -1 {
			selected = append(selected, positiveOrders[fallbackIdx])
		}
	}

	if len(selected) > len(zeroWeightOrders) {
		for i, j := len(zeroWeightOrders), len(selected)-1; i < j; i, j = i+1, j-1 {
			selected[i], selected[j] = selected[j], selected[i]
		}
	}

	totalWeight = 0
	totalValue = 0
	for _, o := range selected {
		totalWeight += o.Weight
		totalValue += o.Value
	}

	return model.DeliveryPlan{
		RobotID:     robotID,
		TotalWeight: totalWeight,
		TotalValue:  totalValue,
		Orders:      selected,
	}, nil
}
