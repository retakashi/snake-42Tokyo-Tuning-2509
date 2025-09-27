import { redirect } from "next/navigation";
import { cookies } from "next/headers";
import axios from "axios";

// サーバーサイドでユーザーの認証状態をチェックする非同期関数
async function checkUserAuthentication() {
  const cookieStore = await cookies();
  
  // セッションクッキーの存在確認
  const sessionCookie = cookieStore.get("session_id");
  if (!sessionCookie) {
    return false;
  }

  // セッションの有効性確認
  try {
    await axios.get(`/api/verify`, {
      headers: {
        Cookie: `session_id=${sessionCookie.value}`
      },
      timeout: 5000, // 5秒のタイムアウト
    });
    return true;
  } catch (error) {
    console.log("認証確認失敗:", error);
    return false;
  }
}

export default async function RootPage() {
  const isLoggedIn = await checkUserAuthentication();
  if (isLoggedIn) {
    redirect("/product");
  } else {
    redirect("/login");
  }
}
