package repository

import (
	"backend/internal/model"
	"context"
	"fmt"
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

	countQuery := "SELECT COUNT(*) FROM products" + filters
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []model.Product{}, 0, nil
	}

	orderClause := fmt.Sprintf(" ORDER BY %s %s, product_id ASC", req.SortField, req.SortOrder)
	query := "SELECT product_id, name, value, weight, image, description FROM products" + filters + orderClause + " LIMIT ? OFFSET ?"
	listArgs := append([]interface{}{}, args...)
	listArgs = append(listArgs, req.PageSize, req.Offset)

	if err := r.db.SelectContext(ctx, &products, query, listArgs...); err != nil {
		return nil, 0, err
	}

	return products, total, nil
}
