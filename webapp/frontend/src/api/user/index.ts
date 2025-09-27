import axios from "axios";

type User = {
  user_id: number;
  user_name: string;
};

export async function login(
  userName: string,
  password: string
): Promise<User | null> {
  const { data: user } = await axios.post<User>("/api/login", {
    user_name: userName,
    password: password,
  });

  console.log("--- ユーザー情報の取得に成功:", user);
  return user;
}

// 認証状態を確認する関数
export async function verifyAuth(): Promise<User | null> {
  try {
    const { data: user } = await axios.get<User>("/api/verify");
    console.log("--- 認証確認に成功:", user);
    return user;
  } catch (error) {
    console.log("--- 認証確認に失敗:", error);
    return null;
  }
}
