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

	// 事前に総注文数を計算し、容量を最適化
	totalOrders := 0
	itemsToProcess := make(map[int]int, len(items))
	for _, item := range items {
		itemsToProcess[item.ProductID] += item.Quantity // 重複商品の数量合算
		totalOrders += item.Quantity
	}

	if len(itemsToProcess) == 0 {
		return []string{}, nil
	}

	var insertedOrderIDs []string

	err := s.store.ExecTx(ctx, func(txStore *repository.Store) error {
		// バッチインサート用の注文リストを事前構築
		orders := make([]model.Order, 0, totalOrders)
		
		// 効率的な注文作成: 構造体を1回作成して使い回し
		for pID, quantity := range itemsToProcess {
			order := model.Order{
				UserID:    userID,
				ProductID: pID,
			}
			for i := 0; i < quantity; i++ {
				orders = append(orders, order)
			}
		}

		// バッチインサートで一括実行（N+1問題解決）
		batchIDs, err := txStore.OrderRepo.CreateBatch(ctx, orders)
		if err != nil {
			return err
		}
		insertedOrderIDs = batchIDs
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
