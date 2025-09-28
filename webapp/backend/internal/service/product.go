package service

import (
	"context"
	"log"

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

		// バルクインサート用の注文リストを構築
		var ordersToInsert []model.Order
		for pID, quantity := range itemsToProcess {
			for i := 0; i < quantity; i++ {
				order := model.Order{
					UserID:    userID,
					ProductID: pID,
				}
				ordersToInsert = append(ordersToInsert, order)
			}
		}

		// バルクインサートを実行
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

func (s *ProductService) FetchProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	products, total, err := s.store.ProductRepo.ListProducts(ctx, userID, req)
	return products, total, err
}
