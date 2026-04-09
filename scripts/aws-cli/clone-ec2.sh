#!/bin/bash
# clone-ec2.sh — 基于现有 EC2 实例快速创建同配置新实例
# 用法: ./clone-ec2.sh <源实例ID> <新实例名称> [实例类型]
# 示例: ./clone-ec2.sh i-0abc123def456 app-server-clone t3.medium

set -euo pipefail

SOURCE_INSTANCE_ID="${1:?请提供源实例 ID，如 i-0abc123def456}"
NEW_NAME="${2:?请提供新实例名称}"
INSTANCE_TYPE="${3:-}"  # 不传则与源实例保持一致

DATE=$(date +%Y%m%d)
AMI_NAME="${NEW_NAME}-${DATE}"

echo "==> 查询源实例信息: $SOURCE_INSTANCE_ID"
INSTANCE_INFO=$(aws ec2 describe-instances \
  --instance-ids "$SOURCE_INSTANCE_ID" \
  --query "Reservations[0].Instances[0].{Type:InstanceType,Subnet:SubnetId,SGs:SecurityGroups[*].GroupId,Key:KeyName}" \
  --output json)

echo "$INSTANCE_INFO"

SOURCE_TYPE=$(echo "$INSTANCE_INFO" | jq -r '.Type')
SUBNET_ID=$(echo "$INSTANCE_INFO" | jq -r '.Subnet')
KEY_NAME=$(echo "$INSTANCE_INFO" | jq -r '.Key')
SG_IDS=$(echo "$INSTANCE_INFO" | jq -r '.SGs | join(" ")')

# 未指定实例类型则沿用源实例
INSTANCE_TYPE="${INSTANCE_TYPE:-$SOURCE_TYPE}"

echo ""
echo "==> 创建 AMI: $AMI_NAME"
AMI_ID=$(aws ec2 create-image \
  --instance-id "$SOURCE_INSTANCE_ID" \
  --name "$AMI_NAME" \
  --description "Clone of $SOURCE_INSTANCE_ID on $DATE" \
  --no-reboot \
  --query "ImageId" \
  --output text)

echo "AMI 创建中: $AMI_ID，等待完成..."
aws ec2 wait image-available --image-ids "$AMI_ID"
echo "AMI 已就绪: $AMI_ID"

echo ""
echo "==> 启动新实例"
NEW_INSTANCE_ID=$(aws ec2 run-instances \
  --image-id "$AMI_ID" \
  --instance-type "$INSTANCE_TYPE" \
  --subnet-id "$SUBNET_ID" \
  --security-group-ids $SG_IDS \
  --key-name "$KEY_NAME" \
  --count 1 \
  --tag-specifications "ResourceType=instance,Tags=[{Key=Name,Value=$NEW_NAME}]" \
  --query "Instances[0].InstanceId" \
  --output text)

echo "新实例启动中: $NEW_INSTANCE_ID，等待运行状态..."
aws ec2 wait instance-running --instance-ids "$NEW_INSTANCE_ID"

echo ""
echo "==> 完成！"
aws ec2 describe-instances \
  --instance-ids "$NEW_INSTANCE_ID" \
  --query "Reservations[0].Instances[0].{ID:InstanceId,State:State.Name,PrivateIP:PrivateIpAddress,PublicIP:PublicIpAddress}" \
  --output table
