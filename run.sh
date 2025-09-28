#!/bin/bash

# ==================================
# リストア・マイグレーション・e2eテスト・負荷試験・採点の順で実施してくれるスクリプト。
# ==================================

check_job_existence() {
    local IS_EXISTS=$1
    local JOB_ID=$2

    if [[ "$IS_EXISTS" == true ]]; then
        echo -e "\n\n===================================================\n\n"
        echo -e "既に負荷試験のリクエストを受け取っています"
        echo -e "負荷試験が完了してから再度リクエストを行ってください"
        echo -e "負荷試験のステータスは下記コマンドで確認できます"
        echo -e "bash get_test_status.sh $JOB_ID"
        echo -e "\n\n===================================================\n\n"
        exit 0
    fi
}

# ファイル名のリストを配列に格納（共通）
FILES=(
    "orders_e2e_10.sql"
    "orders_e2e_1.sql"
    "orders_e2e_2.sql"
    "orders_e2e_3.sql"
    "orders_e2e_4.sql"
    # "orders_e2e_5.sql"
    # "orders_e2e_6.sql"
    # "orders_e2e_7.sql"
    # "orders_e2e_8.sql"
    # "orders_e2e_9.sql"
)

# ランダムファイル選択
RANDOM_INDEX=4

SELECTED_FILE=${FILES[$RANDOM_INDEX]}

# ===============================
# 1. 負荷試験のリクエスト確認（リモートのみ）
# ===============================
if [[ $HOSTNAME == ftt2508-* ]]; then    
    RESPONSE=$(curl -s -G "https://intermediate.ftt2508.dabaas.net/check-existence")
    echo $RESPONSE
    IS_EXISTS=$(echo "$RESPONSE" | jq -r '.isExist')
    JOB_ID=$(echo "$RESPONSE" | jq -r '.jobId')
    check_job_existence $IS_EXISTS $JOB_ID
fi

# ===============================
# 2. リストア・マイグレーション
# ===============================
bash ./restore_and_migration.sh "$SELECTED_FILE"
if [ $? -ne 0 ]; then
    echo -e "採点フロー中断します。"
    exit 1
fi

# ===============================
# 3. E2Eテスト実行
# ===============================
if [[ $HOSTNAME == ftt2508-* ]]; then
    # リモート環境でのE2E実行
    set -euo pipefail
    cd webapp/e2e >/dev/null

    mkdir -p "$(pwd)/tokens"

    docker run --name e2e --rm --network webapp-network \
        -e BASE_URL="https://${HOSTNAME}.ftt2508.dabaas.net" \
        -e USE_DATAINDEX=${RANDOM_INDEX} \
        -v "$(pwd)/tokens:/usr/src/e2e/tokens" \
        -v "$PWD/tsconfig.json:/usr/src/e2e/tsconfig.json:ro" \
        -v "$PWD/tests:/usr/src/e2e/tests" \
        -v "$PWD/playwright.config.ts:/usr/src/e2e/playwright.config.ts:ro" \
        -it 42tokyo2508.azurecr.io/e2e:latest \
        yarn test

    # E2Eコンテナの終了ステータスをチェック
    E2E_EXIT_CODE=$?
    if [ $E2E_EXIT_CODE -ne 0 ]; then
        echo -e "E2Eテストに失敗しました。終了コード: $E2E_EXIT_CODE"
        exit 1
    fi

    cd ../../

    echo "負荷試験のために、データをリセットします。"
    bash ./restore_and_migration.sh

    echo "負荷試験を開始するためのリクエストを送信します。"

    
        
    data="{}"
    RESPONSE=$(curl -s -X POST https://intermediate.ftt2508.dabaas.net/request-load-test \
        -H "Content-Type: application/json" \
        -d "$data")

    # レスポンス処理
    JOB_ID=$(echo "$RESPONSE" | jq -r '.test_id')

    if [ -z "$JOB_ID" ] || [ "$JOB_ID" = "null" ]; then
        echo -e "\n\n===================================================\n\n"
        echo -e "負荷試験のリクエストに失敗しました。メンターに報告してください。"
        echo $RESPONSE
        echo -e "\n\n===================================================\n\n"
        exit 1
    fi

    IS_EXISTS=$(echo "$RESPONSE" | jq -r '.isExists')
    check_job_existence $IS_EXISTS $JOB_ID

    touch ./.da/.initBenchmarker
    echo -e "\n\n===================================================\n\n"
    echo -e "負荷試験のリクエストに成功しました。"
    echo -e "ジョブID: $JOB_ID"
    echo -e "上記のジョブIDをもとに負荷試験のステータスを確認できます"
    echo -e "bash get_test_status.sh $JOB_ID"
    echo -e "\n\n===================================================\n\n"

else
    # ===============================
    # 4. ローカル環境での処理
    # ===============================
    
    # ローカルE2E実行
    (cd webapp/e2e && bash ./run_e2e_test.sh $RANDOM_INDEX)
    
    # ローカル負荷試験実行
    cd ./benchmarker
    docker build --target local-runner -t worker/local-runner:latest worker

    cd worker
    mkdir -p ./scores ./logs
    SCORE=$(docker run --rm \
        --network webapp-network \
        -e TARGET_IP=127.0.0.1 \
        -v "$(pwd)/scores:/app/scores" \
        -v "$(pwd)/logs:/app/logs" \
        worker/local-runner:latest)
    echo "Final Score: $SCORE"
    exit 0
fi