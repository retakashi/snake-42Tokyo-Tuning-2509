#!/bin/bash

#==================================
# リポジトリclone後に最初に実行してもらうスクリプト。
# 初期データのリストア・アプリ環境の構築を実施する。
# 一度だけ実行可能。
# VM側の実行だと引数は必要無いが、ローカル環境だと必須。
# usage ./init.sh [VMのパブリックIPアドレス] [秘密鍵のパス]
#==================================

mkdir -p ./.da

if [[ -e ./.da/.initLock ]]; then
    echo "lockファイルがあるため処理を中断しました。"
    exit 1
fi

# リポジトリ初期化開始
echo "リポジトリの初期化を開始します。"

if [[ $HOSTNAME == ftt2508-* ]]; then
	webUrl="https://$HOSTNAME.ftt2508.dabaas.net/login"
	cp -r /.da/.docker_token ./.da/
	echo "初期データをダウンロードしました。"
	unzip ./.da/remote_restoreSQL.zip -d ./webapp/mysql/init/
elif [ $# -lt 2 ]; then
	echo "引数を2つ指定してください"
    exit 1
else
	webUrl="http://localhost/login"
	vmDomain=$1
	privateKeyPath=$2
	scp -i "$privateKeyPath" azureuser@${vmDomain}:/.da/.docker_token ./.da/.docker_token
fi
curl -L -o ./.da/restoreSQL.zip https://github.com/DreamArts/42Tokyo-Tuning-2509/releases/download/restore_data-v1.0.0/restoreSQL.zip
unzip ./.da/restoreSQL.zip -d ./webapp/mysql/init/ 

# AzureContainerRegistryにログイン
echo "Azure Container Registryにログインします。"
DOCKER_TOKEN=$(<./.da/.docker_token)
docker login -u pull-key -p ${DOCKER_TOKEN} 42tokyo2508.azurecr.io > /dev/null 2>&1 # 

(./restore_and_migration.sh)

if [ $? -ne 0 ]; then	
	echo "初期化に失敗しました。"
	exit 1
else
	touch ./.da/.initLock
	echo -e "\n\n===================================================\n\n"
	echo -e "初期化に成功しました。以下を確認してみてください"
	echo -e "・web画面へアクセスできること(${webUrl})"
	echo -e "・e2eテストの実行。VM環境の場合、負荷試験リクエスト。（ルートディレクトリのrun.shを実行してみてください。）"
	echo -e "\n\n===================================================\n\n"
fi
