package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"
	"context"
	"log"
	"os"
	"sort"
	"strconv"
	"sync"
)

type RobotService struct {
	store        *repository.Store
	cloneEnabled bool
	supplyTarget int
	memPool      *memoryPool
}

type memoryPool struct {
	intSlices    sync.Pool
	pathSlices   sync.Pool
	orderSlices  sync.Pool
}

func newMemoryPool() *memoryPool {
	return &memoryPool{
		intSlices: sync.Pool{
			New: func() interface{} {
				return make([]int, 0, 1024)
			},
		},
		pathSlices: sync.Pool{
			New: func() interface{} {
				return make([]pathNode, 0, 512)
			},
		},
		orderSlices: sync.Pool{
			New: func() interface{} {
				return make([]model.Order, 0, 128)
			},
		},
	}
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
		memPool:      newMemoryPool(),
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
			plan, err = selectOrdersForDeliveryOptimized(ctx, orders, robotID, capacity, s.memPool)
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

type orderWithRatio struct {
	order model.Order
	ratio float64
	index int
}

// 貪欲法による高速な注文選択（小規模ケース用）
func selectOrdersGreedy(ctx context.Context, positiveOrders, zeroWeightOrders []model.Order, robotID string, robotCapacity, baseValue int) (model.DeliveryPlan, error) {
	ratios := make([]orderWithRatio, 0, len(positiveOrders))
	for i, o := range positiveOrders {
		if o.Weight > 0 {
			ratios = append(ratios, orderWithRatio{
				order: o,
				ratio: float64(o.Value) / float64(o.Weight),
				index: i,
			})
		}
	}

	sort.Slice(ratios, func(i, j int) bool {
		return ratios[i].ratio > ratios[j].ratio
	})

	selected := make([]model.Order, 0, len(zeroWeightOrders)+len(ratios))
	selected = append(selected, zeroWeightOrders...)

	currentWeight := 0
	totalValue := baseValue
	for _, r := range ratios {
		if currentWeight + r.order.Weight <= robotCapacity {
			selected = append(selected, r.order)
			currentWeight += r.order.Weight
			totalValue += r.order.Value
		}
	}

	return model.DeliveryPlan{
		RobotID:     robotID,
		TotalWeight: currentWeight,
		TotalValue:  totalValue,
		Orders:      selected,
	}, nil
}

func selectOrdersForDelivery(ctx context.Context, orders []model.Order, robotID string, robotCapacity int) (model.DeliveryPlan, error) {
	return selectOrdersForDeliveryOptimized(ctx, orders, robotID, robotCapacity, nil)
}

// FPTAS実装 - O(n^3/ε)の計算量で(1-ε)近似解を取得
func selectOrdersFPTAS(ctx context.Context, positiveOrders, zeroWeightOrders []model.Order, robotID string, robotCapacity, baseValue int, memPool *memoryPool) (model.DeliveryPlan, error) {
	epsilon := 0.1 // 10%の近似誤差を許容

	if len(positiveOrders) == 0 {
		return model.DeliveryPlan{
			RobotID:     robotID,
			TotalWeight: 0,
			TotalValue:  baseValue,
			Orders:      zeroWeightOrders,
		}, nil
	}

	// 価値の最大値を取得
	maxValue := 0
	for _, order := range positiveOrders {
		if order.Value > maxValue {
			maxValue = order.Value
		}
	}

	// スケーリングファクター計算
	K := int(float64(maxValue) * epsilon / float64(len(positiveOrders)))
	if K == 0 {
		K = 1
	}

	// 価値をスケール
	scaledOrders := make([]model.Order, len(positiveOrders))
	copy(scaledOrders, positiveOrders)
	for i := range scaledOrders {
		scaledOrders[i].Value = scaledOrders[i].Value / K
	}

	// スケールされた問題を解く
	maxScaledValue := 0
	for _, order := range scaledOrders {
		maxScaledValue += order.Value
	}

	// 価値ベースDP
	dp := make([]int, maxScaledValue+1)
	for i := range dp {
		dp[i] = robotCapacity + 1 // 不可能な値で初期化
	}
	dp[0] = 0

	parent := make([][]int, len(scaledOrders))
	for i := range parent {
		parent[i] = make([]int, maxScaledValue+1)
		for j := range parent[i] {
			parent[i][j] = -1
		}
	}

	for i, order := range scaledOrders {
		if err := ctx.Err(); err != nil {
			return model.DeliveryPlan{}, err
		}

		newDp := make([]int, maxScaledValue+1)
		copy(newDp, dp)

		for v := order.Value; v <= maxScaledValue; v++ {
			if dp[v-order.Value] + order.Weight < newDp[v] {
				newDp[v] = dp[v-order.Value] + order.Weight
				parent[i][v] = 1
			}
		}
		dp = newDp
	}

	// 最適解を見つける
	bestValue := 0
	for v := 0; v <= maxScaledValue; v++ {
		if dp[v] <= robotCapacity && v > bestValue {
			bestValue = v
		}
	}

	// 解を復元
	selected := make([]model.Order, 0, len(zeroWeightOrders)+len(positiveOrders))
	selected = append(selected, zeroWeightOrders...)

	v := bestValue
	for i := len(scaledOrders) - 1; i >= 0 && v > 0; i-- {
		if parent[i][v] == 1 {
			selected = append(selected, positiveOrders[i])
			v -= scaledOrders[i].Value
		}
	}

	totalWeight := 0
	totalValue := baseValue
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

// Core Algorithm実装 - 効率的アイテムのみで小問題に分解
func selectOrdersCore(ctx context.Context, positiveOrders, zeroWeightOrders []model.Order, robotID string, robotCapacity, baseValue int, memPool *memoryPool) (model.DeliveryPlan, error) {
	if len(positiveOrders) == 0 {
		return model.DeliveryPlan{
			RobotID:     robotID,
			TotalWeight: 0,
			TotalValue:  baseValue,
			Orders:      zeroWeightOrders,
		}, nil
	}

	// 価値密度でソート
	sort.Slice(positiveOrders, func(i, j int) bool {
		ratioI := float64(positiveOrders[i].Value) / float64(positiveOrders[i].Weight)
		ratioJ := float64(positiveOrders[j].Value) / float64(positiveOrders[j].Weight)
		return ratioI > ratioJ
	})

	// コア集合を特定（効率的なアイテム）
	coreSize := len(positiveOrders) / 3
	if coreSize > 20 {
		coreSize = 20
	}
	if coreSize < 5 {
		coreSize = len(positiveOrders)
	}

	coreItems := positiveOrders[:coreSize]

	// コアアイテムで最適解を計算
	dp := make([]int, robotCapacity+1)
	parent := make([][]bool, len(coreItems))
	for i := range parent {
		parent[i] = make([]bool, robotCapacity+1)
	}

	for i, item := range coreItems {
		if err := ctx.Err(); err != nil {
			return model.DeliveryPlan{}, err
		}

		for w := robotCapacity; w >= item.Weight; w-- {
			if dp[w-item.Weight]+item.Value > dp[w] {
				dp[w] = dp[w-item.Weight] + item.Value
				parent[i][w] = true
			}
		}
	}

	// 最適容量を発見
	bestCap := 0
	for w := 0; w <= robotCapacity; w++ {
		if dp[w] > dp[bestCap] {
			bestCap = w
		}
	}

	// 解を復元
	selected := make([]model.Order, 0, len(zeroWeightOrders)+len(coreItems))
	selected = append(selected, zeroWeightOrders...)

	w := bestCap
	for i := len(coreItems) - 1; i >= 0 && w > 0; i-- {
		if parent[i][w] {
			selected = append(selected, coreItems[i])
			w -= coreItems[i].Weight
		}
	}

	// 残り容量で貪欲法で追加
	remainingCap := robotCapacity
	for _, order := range selected {
		remainingCap -= order.Weight
	}

	for i := coreSize; i < len(positiveOrders) && remainingCap > 0; i++ {
		if positiveOrders[i].Weight <= remainingCap {
			selected = append(selected, positiveOrders[i])
			remainingCap -= positiveOrders[i].Weight
		}
	}

	totalWeight := 0
	totalValue := baseValue
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

func selectOrdersForDeliveryOptimized(ctx context.Context, orders []model.Order, robotID string, robotCapacity int, memPool *memoryPool) (model.DeliveryPlan, error) {
	if robotCapacity <= 0 || len(orders) == 0 {
		return model.DeliveryPlan{RobotID: robotID, Orders: make([]model.Order, 0)}, nil
	}

	// メモリプールから配列を取得
	var filtered []model.Order
	if memPool != nil {
		filtered = memPool.orderSlices.Get().([]model.Order)[:0]
		defer memPool.orderSlices.Put(filtered)
	} else {
		filtered = make([]model.Order, 0, len(orders))
	}

	// フィルタ: 積載量を超える注文は候補外に
	for _, o := range orders {
		if o.Weight <= robotCapacity {
			filtered = append(filtered, o)
		}
	}
	orders = filtered
	if len(orders) == 0 {
		return model.DeliveryPlan{RobotID: robotID, Orders: make([]model.Order, 0)}, nil
	}

	// 重み0と正の重みで分離
	var zeroWeightOrders, positiveOrders []model.Order
	if memPool != nil {
		zeroWeightOrders = memPool.orderSlices.Get().([]model.Order)[:0]
		positiveOrders = memPool.orderSlices.Get().([]model.Order)[:0]
		defer memPool.orderSlices.Put(zeroWeightOrders)
		defer memPool.orderSlices.Put(positiveOrders)
	} else {
		zeroWeightOrders = make([]model.Order, 0, len(orders)/4)
		positiveOrders = make([]model.Order, 0, len(orders))
	}
	totalWeight := 0
	for _, o := range orders {
		if o.Weight == 0 {
			zeroWeightOrders = append(zeroWeightOrders, o)
			continue
		}
		positiveOrders = append(positiveOrders, o)
		totalWeight += o.Weight
	}

	// 選択された注文リスト
	var selected []model.Order
	if memPool != nil {
		selected = memPool.orderSlices.Get().([]model.Order)[:0]
		defer memPool.orderSlices.Put(selected)
	} else {
		selected = make([]model.Order, 0, len(orders))
	}
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

	// アルゴリズム選択戦略
	n := len(positiveOrders)
	capacity := robotCapacity

	// 超小規模: 貪欲法
	if n <= 5 || capacity <= 20 {
		return selectOrdersGreedy(ctx, positiveOrders, zeroWeightOrders, robotID, robotCapacity, totalValue)
	}

	// 中規模: Core Algorithm
	if n <= 50 && capacity <= 200 {
		return selectOrdersCore(ctx, positiveOrders, zeroWeightOrders, robotID, robotCapacity, totalValue, memPool)
	}

	// 大規模: FPTAS
	if n > 100 || capacity > 500 {
		return selectOrdersFPTAS(ctx, positiveOrders, zeroWeightOrders, robotID, robotCapacity, totalValue, memPool)
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

	// 価値密度でソートして効率化
	sort.Slice(positiveOrders, func(i, j int) bool {
		ratioI := float64(positiveOrders[i].Value) / float64(positiveOrders[i].Weight)
		ratioJ := float64(positiveOrders[j].Value) / float64(positiveOrders[j].Weight)
		return ratioI > ratioJ
	})

	// 動的プログラミング用配列をメモリプールから取得
	var bestValue, bestPathIdx []int
	var paths []pathNode

	if memPool != nil {
		bestValue = memPool.intSlices.Get().([]int)[:0]
		bestPathIdx = memPool.intSlices.Get().([]int)[:0]
		paths = memPool.pathSlices.Get().([]pathNode)[:0]
		defer memPool.intSlices.Put(bestValue)
		defer memPool.intSlices.Put(bestPathIdx)
		defer memPool.pathSlices.Put(paths)
	} else {
		bestValue = make([]int, 0, effectiveCap+1)
		bestPathIdx = make([]int, 0, effectiveCap+1)
		paths = make([]pathNode, 0, len(positiveOrders)*effectiveCap/4)
	}

	// サイズ調整
	for len(bestValue) <= effectiveCap {
		bestValue = append(bestValue, 0)
		bestPathIdx = append(bestPathIdx, -1)
	}

	// Branch & Bound用の上界値計算
	upperBound := 0
	remainWeight := effectiveCap
	for i := 0; i < len(positiveOrders) && remainWeight > 0; i++ {
		if positiveOrders[i].Weight <= remainWeight {
			upperBound += positiveOrders[i].Value
			remainWeight -= positiveOrders[i].Weight
		} else {
			// 部分的に取る場合の価値を追加
			upperBound += (positiveOrders[i].Value * remainWeight) / positiveOrders[i].Weight
			break
		}
	}

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

		// Branch & Bound枝刈り: 上界値による早期カット
		currentBest := bestValue[effectiveCap]
		if currentBest > 0 {
			// 現在のアイテム以降で得られる最大価値を計算
			remainingValue := 0
			for j := i; j < len(positiveOrders) && j < i+5; j++ {
				remainingValue += positiveOrders[j].Value
			}
			if currentBest+remainingValue < currentBest*11/10 {
				break
			}
			// 低価値アイテムのスキップ
			if v < currentBest/10 {
				continue
			}
		}

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

	// インデックス配列でパス復元を最適化
	selectedIndices := make([]int, 0, len(positiveOrders))
	for idx := bestPathIdx[bestCap]; idx != -1; idx = paths[idx].prevIdx {
		selectedIndices = append(selectedIndices, paths[idx].itemIndex)
	}

	// 逆順になっているので正順に修正しつつ追加
	for i := len(selectedIndices) - 1; i >= 0; i-- {
		selected = append(selected, positiveOrders[selectedIndices[i]])
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

	// 配列反転処理は不要（すでに正順で追加済み）

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
