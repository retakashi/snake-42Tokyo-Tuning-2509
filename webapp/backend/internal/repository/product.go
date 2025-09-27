package repository

import (
	"backend/internal/model"
	"context"
	"fmt"
	"sync"
)

type ProductRepository struct {
	db DBTX
}

func NewProductRepository(db DBTX) *ProductRepository {
	return &ProductRepository{db: db}
}

// 商品一覧を取得（検索・ソート・ページングはDB側で実施）
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	var (
		products []model.Product
		total    int
	)

	filters := ""
	args := []interface{}{}
	if req.Search != "" {
		filters = " WHERE (name LIKE ? OR description LIKE ?)"
		searchPattern := "%" + req.Search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	orderClause := fmt.Sprintf(" ORDER BY %s %s, product_id ASC", req.SortField, req.SortOrder)
	query := "SELECT product_id, name, value, weight, image, description FROM products" + filters + orderClause + " LIMIT ? OFFSET ?"
	listArgs := append([]interface{}{}, args...)
	listArgs = append(listArgs, req.PageSize, req.Offset)

	errCh := make(chan error, 2)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		countQuery := "SELECT COUNT(*) FROM products" + filters
		if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
			errCh <- err
			cancel()
		}
	}()

	go func() {
		defer wg.Done()
		if err := r.db.SelectContext(ctx, &products, query, listArgs...); err != nil {
			errCh <- err
			cancel()
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return nil, 0, err
		}
	}

	if total == 0 {
		return []model.Product{}, 0, nil
	}

	return products, total, nil
}
