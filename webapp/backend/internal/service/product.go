package service

import (
	"context"
	"log"
	"sync"

	"backend/internal/model"
	"backend/internal/repository"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type ProductService struct {
	store *repository.Store
}

func NewProductService(store *repository.Store) *ProductService {
	return &ProductService{store: store}
}

func (s *ProductService) CreateOrders(ctx context.Context, userID int, items []model.RequestItem) ([]string, error) {
	tracer := otel.Tracer("app/custom")
	ctx, span := tracer.Start(ctx, "CreateOrders")
	defer span.End()
	span.SetAttributes(attribute.Int("user.id", userID), attribute.Int("items.count", len(items)))

	var insertedOrderIDs []string

	err := s.store.ExecTx(ctx, func(txStore *repository.Store) error {
		itemsToProcess := make(map[int]int)
		for _, item := range items {
			if item.Quantity > 0 {
				itemsToProcess[item.ProductID] = item.Quantity
			}
		}
		if len(itemsToProcess) == 0 {
			return nil
		}

		// 全ての注文を事前に構築
		var ordersToInsert []model.Order
		for pID, quantity := range itemsToProcess {
			for i := 0; i < quantity; i++ {
				ordersToInsert = append(ordersToInsert, model.Order{
					UserID:    userID,
					ProductID: pID,
				})
			}
		}

		// 大量の注文の場合、バッチに分けて並列処理
		const batchSize = 1000
		if len(ordersToInsert) > batchSize {
			return s.createOrdersInBatches(ctx, txStore, ordersToInsert, &insertedOrderIDs)
		}

		// 少量の場合は単純なバルクインサート
		orderIDs, err := txStore.OrderRepo.CreateBulk(ctx, ordersToInsert)
		if err != nil {
			return err
		}
		insertedOrderIDs = orderIDs
		return nil
	})

	if err != nil {
		return nil, err
	}
	log.Printf("Created %d orders for user %d", len(insertedOrderIDs), userID)
	return insertedOrderIDs, nil
}

// createOrdersInBatches processes large order batches in parallel
func (s *ProductService) createOrdersInBatches(ctx context.Context, txStore *repository.Store, orders []model.Order, insertedOrderIDs *[]string) error {
	const batchSize = 1000
	const maxConcurrency = 4

	// バッチに分割
	var batches [][]model.Order
	for i := 0; i < len(orders); i += batchSize {
		end := i + batchSize
		if end > len(orders) {
			end = len(orders)
		}
		batches = append(batches, orders[i:end])
	}

	// 並列処理用のセマフォ
	semaphore := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allOrderIDs []string
	errCh := make(chan error, len(batches))

	for _, batch := range batches {
		wg.Add(1)
		go func(batchOrders []model.Order) {
			defer wg.Done()
			semaphore <- struct{}{} // セマフォ取得
			defer func() { <-semaphore }() // セマフォ解放

			batchOrderIDs, err := txStore.OrderRepo.CreateBulk(ctx, batchOrders)
			if err != nil {
				errCh <- err
				return
			}

			mu.Lock()
			allOrderIDs = append(allOrderIDs, batchOrderIDs...)
			mu.Unlock()
		}(batch)
	}

	wg.Wait()
	close(errCh)

	// エラーをチェック
	for err := range errCh {
		if err != nil {
			return err
		}
	}

	*insertedOrderIDs = allOrderIDs
	return nil
}

func (s *ProductService) FetchProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	products, total, err := s.store.ProductRepo.ListProducts(ctx, userID, req)
	return products, total, err
}
