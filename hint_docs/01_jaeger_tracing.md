## ヒント1：トレーシングとJaeger（イェーガー）の使い方

---

### 目次

 - [1 トレーシングってなに？](#1-トレーシングってなに)   
 - [2 Jaegerってどんなツール？](#3-jaegerってどんなツール)  
 - [3 Jaegerの使い方](#4-jaegerの使い方)  

 ---

### 1 トレーシングってなに？

トレーシングは、**アプリケーションの中で「どんな処理が、どのくらい時間がかかっているか」を調べる方法**です。

例えば、ページを表示するときに時間がかかっているとします。そのとき、トレーシングを使うと：

- どこで時間がかかっているのか？
- どの関数が呼ばれているのか？
- どのデータベースのクエリが遅いのか？

などを、**可視化**することができます。

#### スパン（Span）とは

トレーシングで出てくる「スパン（Span）」は、**処理の単位（1つの操作）を表すタイムスタンプ付きの記録**です。例えば「HTTP リクエストの受信」「特定の DB クエリの実行」「外部 API 呼び出し」などがそれぞれスパンになります。スパンには開始時刻・終了時刻・処理名・追加の属性（例: クエリの SQL、ステータス）などが含まれ、複数のスパンをつなげることでリクエスト全体の流れ（トレース）を再現できます。
これにより、どのスパンがボトルネックかを特定して優先的にチューニングできます。

---

### 2 Jaegerってどんなツール？

[Jaeger（イェーガー）](https://www.jaegertracing.io/) は、トレーシングの情報を集めて、**画面で表示してくれるツール**です。

Jaegerを使うと：

- 処理の流れをグラフで見られる  
- 各処理にかかった時間が確認できる 
- 遅い処理の場所が見つけやすくなる

というメリットがあります。

---

### 3 Jaegerの使い方

このプロジェクトでは OpenTelemetry（OTel）を使ってトレーシングを収集し、Jaeger に送信する構成を想定しています。ルーティング（HTTP）には `otelchi`、SQL 層には `otelsql` を使っており、これらは自動的にスパンを作成します。つまり、アプリ内の主要なリクエスト経路や SQL 実行は既にトレースされます。

以下は、ローカルで Jaeger に接続してトレースを収集するための最小限の手順と、トレーシングを一時的に無効化する方法の両方を示します。

最小限の設定（要点）: `webapp/backend/cmd/main.go` に、ファイル先頭の import ブロックと `main` 内の初期化コードを一緒に書いてください。以下のコード例はそのまま `main.go` に入れます。

#### 最小の初期化コード例（`webapp/backend/cmd/main.go` の `main` 内）
```go
package main

import (
   "context"
   "log"
   // ...既存のインポート...
   "backend/internal/telemetry"
)

func main() {
   // アプリ起動前に telemetry を初期化
   shutdown, err := telemetry.Init(context.Background())
   if err != nil {
      log.Printf("telemetry init failed: %v, continuing without telemetry", err)
   } else {
      defer func() { _ = shutdown(context.Background()) }()
   }

   // ...既存の main の処理...
}
```

- `telemetry.Init` を呼び出すことで、OpenTelemetry の初期設定（TracerProviderやエクスポータなど）を行います。これにより、`otelchi` / `otelsql` が生成するスパンが Jaeger に送信されます。
- アプリの他の部分でカスタムスパンを作成したい場合は、OpenTelemetry の Tracer を使って明示的に Span を作成してください。

---

#### カスタムスパン（otelchi/otelsql で自動計測されない処理）と Jaeger UI

下記のコードは `webapp/backend/internal/service/product.go #CreateOrders` での実装例です。

```go
import (
   "context"

   "backend/internal/model"
   "backend/internal/repository"

   "go.opentelemetry.io/otel"
   "go.opentelemetry.io/otel/attribute"
)

// 既存のコード

func (s *ProductService) CreateOrders(ctx context.Context, userID int, items []model.RequestItem) ([]string, error) {
   tracer := otel.Tracer("app/custom")
   ctx, span := tracer.Start(ctx, "CreateOrders")
   defer span.End()
   span.SetAttributes(attribute.Int("user.id", userID), attribute.Int("items.count", len(items)))

   // 既存のコード
}
```

---

#### Jaeger UI にアクセスする場所:

- ローカル環境: http://localhost/jaeger/search 
- VM 環境: 
https://{VMのドメイン名}.ftt2508.dabaas.net/jaeger/search
