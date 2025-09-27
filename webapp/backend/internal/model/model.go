package model

import (
	"database/sql"
	"time"
)

type User struct {
	UserID       int    `db:"user_id"`
	PasswordHash string `db:"password_hash"`
	UserName     string `db:"user_name"`
}

type Product struct {
	ProductID   int    `db:"product_id"   json:"product_id"`
	Name        string `db:"name"         json:"name"`
	Value       int    `db:"value"        json:"value"`
	Weight      int    `db:"weight"       json:"weight"`
	Image       string `db:"image"        json:"image"`
	Description string `db:"description"  json:"description"`
}

type Order struct {
	OrderID       int64        `db:"order_id"        json:"order_id"`
	UserID        int          `db:"user_id"         json:"user_id"`
	ProductID     int          `db:"product_id"      json:"product_id"`
	ProductName   string       `db:"product_name"    json:"product_name"`
	ShippedStatus string       `db:"shipped_status"  json:"shipped_status"`
	Weight        int          `db:"weight"          json:"weight"`
	Value         int          `db:"value"           json:"value"`
	CreatedAt     time.Time    `db:"created_at"      json:"created_at"`
	ArrivedAt     sql.NullTime `db:"arrived_at"      json:"arrived_at"`
}

type DeliveryPlan struct {
	RobotID     string  `json:"robot_id"`
	TotalWeight int     `json:"total_weight"`
	TotalValue  int     `json:"total_value"`
	Orders      []Order `json:"orders"`
}

type LoginRequest struct {
	UserName string `json:"user_name"`
	Password string `json:"password"`
}

type CreateOrderRequest struct {
	Items []RequestItem `json:"items"`
}

type RequestItem struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

type UpdateOrderStatusRequest struct {
	OrderID   int64  `json:"order_id"`
	NewStatus string `json:"new_status"`
}

type LoginResponse struct {
	UserID   int    `json:"user_id"`
	UserName string `json:"user_name"`
}

type ListRequest struct {
	Search    string `json:"search"`
	Type      string `json:"type"`
	Page      int    `json:"page"`
	PageSize  int    `json:"page_size"`
	SortField string `json:"sort_field"`
	SortOrder string `json:"sort_order"`
	Offset    int    `json:"-"`
}
