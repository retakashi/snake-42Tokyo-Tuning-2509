package main

import (
	"backend/internal/server"
	"log"
)

func main() {
	// トレース機能を無効化してパフォーマンス最適化
	srv, dbConn, err := server.NewServer()
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}
	if dbConn != nil {
		defer dbConn.Close()
	}

	srv.Run()
}
