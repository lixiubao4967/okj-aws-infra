#!/bin/bash
# 将 OpenLDAP 导出的 LDIF 文件转换为 AD 导入用的 CSV 格式
# 用法: ./ldap-to-ad-csv.sh <ldap_users.ldif> > ad_users.csv
# 示例: ./ldap-to-ad-csv.sh ldap_users.ldif > ad_users.csv
#
# 前置步骤 — 从 OpenLDAP 导出用户:
#   ldapsearch -x -H ldap://your-ldap-server \
#     -b "ou=People,dc=example,dc=com" \
#     -D "cn=admin,dc=example,dc=com" -W \
#     "(objectClass=posixAccount)" \
#     uid cn sn givenName mail > ldap_users.ldif
set -euo pipefail

INPUT="${1:?用法: $0 <ldap_users.ldif>}"

if [[ ! -f "$INPUT" ]]; then
  echo "错误: 文件不存在: $INPUT" >&2
  exit 1
fi

echo "SamAccountName,GivenName,Surname,DisplayName,EmailAddress,Password"

awk '
BEGIN { OFS="," }
/^dn:/ { uid=""; cn=""; sn=""; given=""; mail="" }
/^uid:/ { gsub(/^uid: */, ""); uid=$0 }
/^cn:/ { gsub(/^cn: */, ""); cn=$0 }
/^sn:/ { gsub(/^sn: */, ""); sn=$0 }
/^givenName:/ { gsub(/^givenName: */, ""); given=$0 }
/^mail:/ { gsub(/^mail: */, ""); mail=$0 }
/^$/ {
  if (uid != "") {
    if (sn == "") sn = cn
    if (given == "") given = cn
    pass = "TempP@ss" uid "2024!"
    print uid, given, sn, cn, mail, pass
  }
}
END {
  if (uid != "") {
    if (sn == "") sn = cn
    if (given == "") given = cn
    pass = "TempP@ss" uid "2024!"
    print uid, given, sn, cn, mail, pass
  }
}
' "$INPUT"
