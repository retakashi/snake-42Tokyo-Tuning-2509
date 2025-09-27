package repository

import (
	"backend/internal/model"
	"context"
	"sync"
)

type ProductRepository struct {
	db DBTX
}

func NewProductRepository(db DBTX) *ProductRepository {
	return &ProductRepository{db: db}
}

// 商品一覧をDB側でページングして取得し、並列でCOUNTクエリを実行する
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	var products []model.Product
	var total int
	var wg sync.WaitGroup
	var productErr, countErr error

	// 商品データ取得（goroutine）
	wg.Add(1)
	go func() {
		defer wg.Done()
		baseQuery := `
			SELECT product_id, name, value, weight, image, description
			FROM products
		`
		args := []interface{}{}

		if req.Search != "" {
			baseQuery += " WHERE (name LIKE ? OR description LIKE ?)"
			searchPattern := "%" + req.Search + "%"
			args = append(args, searchPattern, searchPattern)
		}

		baseQuery += " ORDER BY " + req.SortField + " " + req.SortOrder + " , product_id ASC"
		baseQuery += " LIMIT ? OFFSET ?"
		args = append(args, req.PageSize, req.Offset)

		productErr = r.db.SelectContext(ctx, &products, baseQuery, args...)
	}()

	// 総数取得（goroutine）
	wg.Add(1)
	go func() {
		defer wg.Done()
		countQuery := `
			SELECT COUNT(*) FROM products
		`
		countArgs := []interface{}{}

		if req.Search != "" {
			countQuery += " WHERE (name LIKE ? OR description LIKE ?)"
			countArgs = append(countArgs, "%"+req.Search+"%", "%"+req.Search+"%")
		}
		countErr = r.db.GetContext(ctx, &total, countQuery, countArgs...)
	}()

	// 両方のgoroutineの完了を待つ
	wg.Wait()

	// エラーチェック
	if productErr != nil {
		return nil, 0, productErr
	}
	if countErr != nil {
		return nil, 0, countErr
	}

	return products, total, nil
}
