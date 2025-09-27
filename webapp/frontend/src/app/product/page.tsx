"use client";

import React, { useEffect, useState, useCallback } from "react";
import CustomDataGrid from "../../components/CustomDataGrid";
import {
  GridColDef,
  GridRenderCellParams,
  GridSortModel,
} from "@mui/x-data-grid";
import axios from "axios";
import {
  Box,
  Container,
  Typography,
  TextField,
  Button,
  Paper,
} from "@mui/material";
import { useRouter } from "next/navigation";
import { verifyAuth } from "@/api/user";

type Product = {
  product_id: number;
  user_id: number;
  name: string;
  value: number;
  weight: number;
  image: string;
  description: string;
};

export default function ProductListPage() {
  const [products, setProducts] = useState<Product[]>([]);
  const [totalCount, setTotalCount] = useState<number>(0);
  const [quantities, setQuantities] = useState<{ [id: number]: string }>({});
  const [searchQuery, setSearchQuery] = useState<string>("");
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [page, setPage] = useState<number>(1);
  const [pageSize, setPageSize] = useState<number>(20);
  const [sortModel, setSortModel] = useState<GridSortModel>([
    { field: "product_id", sort: "asc" },
  ]);
  const [pendingSearchQuery, setPendingSearchQuery] = useState<string>("");
  const [currentUser, setCurrentUser] = useState<{ user_id: number; user_name: string } | null>(null);

  const router = useRouter();

  // 認証状態を確認
  useEffect(() => {
    const checkAuth = async () => {
      const user = await verifyAuth();
      if (!user) {
        // 認証が無効な場合、ログインページにリダイレクト
        router.push("/login");
      } else {
        setCurrentUser(user);
      }
    };
    checkAuth();
  }, [router]);

  // バックエンドAPIから商品一覧を取得（サーバーサイドソート・ページング対応）
  const fetchProducts = useCallback(
    (
      pageNum: number = page,
      size: number = pageSize,
      model: GridSortModel = sortModel
    ) => {
      setIsLoading(true);
      const sortF = model[0]?.field ?? "product_id";
      const sortO = model[0]?.sort ?? "asc";
      axios
        .post("/api/v1/product", {
          search: searchQuery,
          page: pageNum,
          page_size: size,
          sort_field: sortF,
          sort_order: sortO,
        })
        .then((response) => {
          setProducts(response.data.data || []);
          setTotalCount(response.data.total || 0);
        })
        .catch((error) => {
          console.error("商品データの取得に失敗しました:", error);
          setProducts([]);
        })
        .finally(() => setIsLoading(false));
    },
    [page, pageSize, searchQuery, sortModel]
  );

  useEffect(() => {
    fetchProducts(page, pageSize, sortModel);
  }, [page, pageSize, searchQuery, sortModel, fetchProducts]);

  const handleQuantityChange = (id: number, value: string) => {
    setQuantities((prev) => ({
      ...prev,
      [id]: value,
    }));
  };

  const handleSubmit = async () => {
    const selected = products
      .filter((product) => {
        const quantity = Number(quantities[product.product_id]) || 0;
        return quantity > 0;
      })
      .map((product) => ({
        product_id: product.product_id,
        quantity: Number(quantities[product.product_id]),
      }));

    if (selected.length === 0) {
      alert("商品を選択してください。");
      return;
    }

    try {
      await axios.post("/api/v1/product/post", {
        items: selected,
      });
      alert("注文が正常に送信されました。");
      setQuantities({});
    } catch (error) {
      console.error("エラー:", error);
      alert("注文の送信に失敗しました。");
    }
  };

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    setSearchQuery(pendingSearchQuery);
    setPage(1);
  };

  const handleReset = () => {
    setPendingSearchQuery("");
    setSearchQuery("");
    setPage(1);
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setPendingSearchQuery(e.target.value);
  };

  const handlePageChange = (newPage: number) => {
    setPage(newPage + 1);
  };

  const handlePageSizeChange = (newSize: number) => {
    setPageSize(newSize);
    setPage(1);
  };

  const handleSortModelChange = (model: GridSortModel) => {
    if (model.length === 0) return;
    setSortModel(model);
  };

  const getImageUrl = (imagePath: string) => {
    if (!imagePath) return "/default-product.png";
    return `/api/v1/image?path=${encodeURIComponent(imagePath)}`;
  };

  const columns: GridColDef[] = [
    {
      field: "image",
      headerName: "画像",
      flex: 0.8,
      minWidth: 100,
      headerAlign: "center",
      align: "center",
      sortable: false,
      filterable: false,
      renderCell: (params: GridRenderCellParams) => (
        <Box
          component="img"
          src={getImageUrl(params.row.image)}
          alt={params.row.name}
          sx={{
            width: 60,
            height: 60,
            objectFit: "cover",
            borderRadius: 1,
            border: "1px solid #ddd",
          }}
          onError={(e) => {
            (e.target as HTMLImageElement).src = "/default-product.png";
          }}
        />
      ),
    },
    {
      field: "product_id",
      headerName: "商品ID",
      flex: 0.5,
      minWidth: 80,
      headerAlign: "left",
      align: "left",
    },
    {
      field: "name",
      headerName: "商品名",
      flex: 2,
      minWidth: 200,
    },
    {
      field: "value",
      headerName: "価格",
      flex: 1,
      minWidth: 100,
      headerAlign: "right",
      align: "right",
      renderCell: (params) => {
        if (!params.value && params.value !== 0) return "";
        try {
          return `¥${Number(params.value).toLocaleString()}`;
        } catch {
          return "¥0";
        }
      },
    },
    {
      field: "weight",
      headerName: "重量",
      flex: 0.8,
      minWidth: 80,
      headerAlign: "right",
      align: "right",
      renderCell: (params) => {
        if (!params.value && params.value !== 0) return "";
        try {
          return `${Number(params.value)}g`;
        } catch {
          return "0g";
        }
      },
    },
    {
      field: "description",
      headerName: "説明",
      flex: 2.5,
      minWidth: 250,
      sortable: false,
      filterable: false,
    },
    {
      field: "quantity",
      headerName: "数量",
      flex: 1,
      minWidth: 120,
      headerAlign: "center",
      align: "center",
      sortable: false,
      filterable: false,
      renderCell: (params: GridRenderCellParams) => {
        const productId = params.row.product_id;
        return (
          <TextField
            size="small"
            slotProps={{
              input: {
                inputProps: {
                  min: 0,
                  style: { textAlign: "center" },
                  pattern: "[0-9]*",
                },
              },
            }}
            value={quantities[productId] ?? ""}
            onChange={(e) => {
              const value = e.target.value;
              if (value === "" || /^\d+$/.test(value)) {
                handleQuantityChange(productId, value);
              }
            }}
            placeholder="0"
            sx={{ width: 80 }}
            disabled={isLoading}
          />
        );
      },
    },
  ];

  return (
    <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
      <Typography variant="h4" component="h1" gutterBottom>
        商品一覧
      </Typography>

      {/* Ordersページへ遷移するボタンを追加 */}
      <Box sx={{ mb: 2, display: "flex", justifyContent: "flex-end" }}>
        <Button
          variant="outlined"
          color="secondary"
          onClick={() => router.push("/orders")}
        >
          注文一覧ページへ
        </Button>
      </Box>

      <Paper elevation={1} sx={{ p: 2, mb: 3 }}>
        <form onSubmit={handleSearch}>
          <Box sx={{ display: "flex", gap: 2, alignItems: "center" }}>
            <TextField
              placeholder="商品名または説明で検索..."
              value={pendingSearchQuery}
              onChange={handleInputChange}
              size="small"
              sx={{ minWidth: 300, flex: 1 }}
              disabled={isLoading}
            />
            <Button type="submit" variant="contained" disabled={isLoading}>
              検索
            </Button>
            <Button
              variant="outlined"
              onClick={handleReset}
              disabled={isLoading}
            >
              リセット
            </Button>
          </Box>
        </form>
      </Paper>

      <Box sx={{ height: 650, width: "100%" }}>
        <CustomDataGrid
          rows={products}
          columns={columns}
          getRowId={(row) => row.product_id}
          loading={isLoading}
          rowCount={totalCount}
          paginationModel={{ page: page - 1, pageSize: pageSize }}
          paginationMode="server"
          onPaginationModelChange={({ page: newPage, pageSize: newSize }) => {
            if (newSize !== pageSize) {
              handlePageSizeChange(newSize);
            } else if (newPage !== page - 1) {
              handlePageChange(newPage);
            }
          }}
          sortingMode="server"
          sortModel={sortModel}
          onSortModelChange={handleSortModelChange}
          sortingOrder={["asc", "desc"]}
        />
        {/* 送信ボタンを表の下に移動 */}
        <Paper elevation={1} sx={{ p: 2, mt: 2 }}>
          <Box
            sx={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
            }}
          >
            <Typography variant="body2" color="text.secondary">
              数量を入力して「注文送信」ボタンを押してください
            </Typography>
            <Button
              variant="contained"
              color="primary"
              size="large"
              onClick={handleSubmit}
              disabled={isLoading}
            >
              注文送信
            </Button>
          </Box>
        </Paper>
      </Box>
    </Container>
  );
}
