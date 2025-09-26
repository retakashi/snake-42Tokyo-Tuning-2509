import { check, fail } from "k6";
import http from "k6/http";
import { Counter } from "k6/metrics";
import { createRobotUrl } from "./utils.js";

const bench_robot_success_count = new Counter("bench_robot_success_count");

const bench_robot_getDeliveryPlan_fail_count = new Counter(
  "bench_robot_getDeliveryPlan_fail_count"
);
const bench_robot_updateOrdersStatus_fail_count = new Counter(
  "bench_robot_updateOrdersStatus_fail_count"
);

// テストシナリオ
export function robotAPIScenario() {
  const headers = {
    "X-API-KEY": "test-robot-key",
    "Content-Type": "application/json",
  };

  const orderIds = getDeliveryPlan(headers, "");

  for (const order_id of orderIds) {
    updateOrderStatus(headers, order_id, "");
  }

  bench_robot_success_count.add(1);
}

// ナップサック計算API呼び出し
function getDeliveryPlan(headers, mode = "") {
  const url = createRobotUrl("/delivery-plan");
  const finalUrl = `${url}?capacity=100`;

  const robot_res = http.get(finalUrl, {
    headers: headers,
  });

  const robotCheck = check(robot_res, {
    "is status 200": (r) => r.status === 200,
  });

  if (!robotCheck) {
    bench_robot_getDeliveryPlan_fail_count.add(1);
    fail("Failed to get delivery plan");
  }

  // order_id抽出（複数対応）
  let orderIds = [];
  try {
    const body = JSON.parse(robot_res.body);

    if (!body || !Array.isArray(body.orders)) {
      return orderIds;
    }

    for (const order of body.orders) {
      if (order && order.order_id) {
        orderIds.push(order.order_id);
      }
    }
  } catch (e) {
    bench_robot_getDeliveryPlan_fail_count.add(1);
    fail(
      `Failed to parse delivery plan response: ${e} - body: ${robot_res.body}`
    );
  }

  return orderIds;
}

// getDeliveryPlanで既にステータス変更まで入ってそう（shipping→delivering）
// ステータス更新API呼び出し
function updateOrderStatus(headers, order_id, mode = "") {
  const url2 = createRobotUrl("/orders/status");

  const robotUpdateRes = http.patch(
    url2,
    JSON.stringify({ order_id: order_id, new_status: "completed" }),
    {
      headers: headers,
    }
  );

  const robotUpdateCheck = check(robotUpdateRes, {
    "is status 200": (r) => r.status === 200,
    "body is updated message": (r) => r.body === "Order status updated",
  });

  if (!robotUpdateCheck) {
    bench_robot_updateOrdersStatus_fail_count.add(1);
    fail("Failed to update order status");
  }
}
