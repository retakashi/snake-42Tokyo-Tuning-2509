-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。
CREATE UNIQUE INDEX idx_users_user_name ON `users` (`user_name`);

CREATE INDEX idx_orders_user_id_created_at ON orders (user_id, created_at);
CREATE INDEX idx_orders_shipped_status_product ON orders (shipped_status, product_id);