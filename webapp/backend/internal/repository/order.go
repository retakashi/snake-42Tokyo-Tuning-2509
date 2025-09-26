package repository

import (
	"backend/internal/model"
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type OrderRepository struct {
	db DBTX
}

func NewOrderRepository(db DBTX) *OrderRepository {
	return &OrderRepository{db: db}
}

// 注文を作成し、生成された注文IDを返す
func (r *OrderRepository) Create(ctx context.Context, order *model.Order) (string, error) {
	query := `INSERT INTO orders (user_id, product_id, shipped_status, created_at) VALUES (?, ?, 'shipping', NOW())`
	result, err := r.db.ExecContext(ctx, query, order.UserID, order.ProductID)
	if err != nil {
		return "", err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", id), nil
}

// 複数の注文IDのステータスを一括で更新
// 主に配送ロボットが注文を引き受けた際に一括更新をするために使用
func (r *OrderRepository) UpdateStatuses(ctx context.Context, orderIDs []int64, newStatus string) error {
	if len(orderIDs) == 0 {
		return nil
	}
	query, args, err := sqlx.In("UPDATE orders SET shipped_status = ? WHERE order_id IN (?)", newStatus, orderIDs)
	if err != nil {
		return err
	}
	query = r.db.Rebind(query)
	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}

// CountShipping returns the current number of shipping orders.
func (r *OrderRepository) CountShipping(ctx context.Context) (int, error) {
	const query = "SELECT COUNT(*) FROM orders WHERE shipped_status = 'shipping'"
	var total int
	if err := r.db.GetContext(ctx, &total, query); err != nil {
		return 0, err
	}
	return total, nil
}

// CloneAsShipping duplicates specified orders as new shipping entries to keep supply available.
func (r *OrderRepository) CloneAsShipping(ctx context.Context, orderIDs []int64) error {
	if len(orderIDs) == 0 {
		return nil
	}
	query, args, err := sqlx.In(
		"INSERT INTO orders (user_id, product_id, shipped_status, created_at) "+
			"SELECT user_id, product_id, 'shipping', NOW() FROM orders WHERE order_id IN (?)",
		orderIDs,
	)
	if err != nil {
		return err
	}
	query = r.db.Rebind(query)
	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}

// 配送中(shipped_status:shipping)の注文一覧を取得
func (r *OrderRepository) GetShippingOrders(ctx context.Context) ([]model.Order, error) {
	var orders []model.Order
	query := `
        SELECT
            o.order_id,
            p.weight,
            p.value
        FROM orders o
        JOIN products p ON o.product_id = p.product_id
        WHERE o.shipped_status = 'shipping'
    `
	err := r.db.SelectContext(ctx, &orders, query)
	return orders, err
}

// 注文履歴一覧を取得
func (r *OrderRepository) ListOrders(ctx context.Context, userID int, req model.ListRequest) ([]model.Order, int, error) {
	filters := []string{"o.user_id = ?"}
	args := []interface{}{userID}
	if req.Search != "" {
		pattern := "%" + req.Search + "%"
		if req.Type == "prefix" {
			pattern = req.Search + "%"
		}
		filters = append(filters, "p.name LIKE ?")
		args = append(args, pattern)
	}

	whereClause := ""
	if len(filters) > 0 {
		whereClause = " WHERE " + strings.Join(filters, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM orders o JOIN products p ON o.product_id = p.product_id" + whereClause
	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []model.Order{}, 0, nil
	}

	orderClause := fmt.Sprintf(" ORDER BY %s %s", req.SortField, req.SortOrder)
	if req.SortField != "o.order_id" {
		orderClause += ", o.order_id ASC"
	}

	query := fmt.Sprintf(`
		SELECT o.order_id, o.user_id, o.product_id, p.name AS product_name, o.shipped_status, o.created_at, o.arrived_at, p.weight, p.value
		FROM orders o
		JOIN products p ON o.product_id = p.product_id%s%s
		LIMIT ? OFFSET ?`, whereClause, orderClause)
	listArgs := append([]interface{}{}, args...)
	listArgs = append(listArgs, req.PageSize, req.Offset)

	var orders []model.Order
	if err := r.db.SelectContext(ctx, &orders, query, listArgs...); err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}
