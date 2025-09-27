import { test, expect } from "@playwright/test";

const ROBOT_API_KEY = process.env.ROBOT_API_KEY || "test-robot-key";

test("ロボット: delivery-plan → delivering → status更新で completed", async ({
  page,
}) => {
  // 1) ログイン → /product
  await page.goto("/login");
  await page.getByLabel(/UserName|ユーザー名/i).fill("user001");
  await page.getByLabel(/Password|パスワード/i).fill("password");
  await page.getByRole("button", { name: /ログイン|Sign in/i }).click();
  await expect(page).toHaveURL(/\/product$/);
  await expect(page.getByRole("heading", { name: "商品一覧" })).toBeVisible();

  // 初回ロード：行が出るまで
  const nameCells = page.locator('[role="gridcell"][data-field="name"]');
  await expect(nameCells).not.toHaveCount(0, { timeout: 15000 });

  // 任意の1行をターゲットに（同じ行の product_id と name を取得）
  const firstNameCell = nameCells.first();
  const targetRow = firstNameCell.locator("xpath=ancestor::*[@role='row'][1]");
  const productName = (await firstNameCell.textContent())?.trim() ?? "";
  expect(productName.length).toBeGreaterThan(0);

  const productIdText =
    (
      await targetRow
        .locator('[role="gridcell"][data-field="product_id"]')
        .textContent()
    )?.trim() ?? "";
  const productId = Number(productIdText);
  expect(Number.isFinite(productId)).toBeTruthy();

  // 数量を入れて注文送信
  await targetRow.locator('input[placeholder="0"]').fill("1");
  const alertP = new Promise<void>((resolve) => {
    page.once("dialog", async (d) => {
      expect(d.type()).toBe("alert");
      expect(d.message()).toBe("注文が正常に送信されました。");
      await d.accept();
      resolve();
    });
  });
  const orderPost = page.waitForResponse(
    (r) =>
      r.url().includes("/api/v1/product/post") &&
      r.request().method() === "POST"
  );
  await page.getByRole("button", { name: "注文送信" }).click();
  expect((await orderPost).status()).toBe(200);
  await alertP;

  // 2) ロボット: delivery-plan を叩く（capacityを十分に大きく）
  const planResp = await page.request.get("/api/robot/delivery-plan", {
    headers: { "X-API-KEY": ROBOT_API_KEY },
    params: { capacity: 100000 }, // g 単位想定。十分大きく全件対象にする
  });
  expect(planResp.ok()).toBeTruthy();
  const plan = await planResp.json();
  expect(Array.isArray(plan?.orders)).toBeTruthy();
  expect(plan.orders.length).toBeGreaterThan(0);

  const picked = plan.orders.find(
    (o: any) => Number(o.product_id) === productId
  );
  expect(
    picked,
    "delivery-plan に今注文した商品が含まれていること"
  ).toBeTruthy();
  const orderId = Number(picked.order_id);
  expect(Number.isFinite(orderId)).toBeTruthy();

  // 3) Ordersで delivering 反映を待つ（検索して該当行のチップ確認）
  await page.goto("/orders");
  await expect(page.getByRole("heading", { name: "注文一覧" })).toBeVisible();

  // 検索（部分一致のままでOK）
  await page
    .getByPlaceholder("商品名またはステータスで検索...")
    .fill(productName.slice(0, 8));
  const [searchRes1] = await Promise.all([
    page.waitForResponse(
      (r) =>
        r.url().includes("/api/v1/orders") &&
        r.request().method() === "POST" &&
        r.status() === 200
    ),
    page.getByRole("button", { name: "検索" }).click(),
  ]);
  expect((await searchRes1.json())?.data).toBeDefined();

  const ordersRow = page
    .locator('[role="row"]')
    .filter({ has: page.getByText(productName, { exact: false }) })
    .first();

  const waitForStatusChip = async (label: string, timeoutMs = 5000) => {
    await expect(async () => {
      const [res] = await Promise.all([
        page.waitForResponse(
          (r) =>
            r.url().includes("/api/v1/orders") &&
            r.request().method() === "POST" &&
            r.status() === 200
        ),
        page.getByRole("button", { name: "検索" }).click(),
      ]);
      void res;

      await expect(ordersRow).toBeVisible({ timeout: 300 });
      await expect(ordersRow.getByText(label)).toBeVisible({ timeout: 300 });
    }).toPass({ timeout: timeoutMs, intervals: [200, 400, 800] });
  };

  // delivering 反映を待機
  await waitForStatusChip("配送中", 5000);
  expect(
    await ordersRow.getByText("配送中").isVisible(),
    "delivery-plan 後に配送中になること"
  ).toBeTruthy();

  // 4) status更新APIで completed に
  const updResp = await page.request.patch("/api/robot/orders/status", {
    headers: {
      "X-API-KEY": ROBOT_API_KEY,
      "Content-Type": "application/json",
    },
    data: { order_id: orderId, new_status: "completed" },
  });
  expect(updResp.ok()).toBeTruthy();

  // completed 反映待ち（同様に短ポーリング）
  await waitForStatusChip("配送完了", 7000);
  expect(
    await ordersRow.getByText("配送完了").isVisible(),
    "status更新後に配送完了になること"
  ).toBeTruthy();
});
