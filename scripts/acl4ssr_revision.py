"""查询 ACL4SSR/ACL4SSR 的最新提交，供构建脚本注入到 -ldflags。

用法：
    python scripts/acl4ssr_revision.py          # 打印 revision
    python scripts/acl4ssr_revision.py --check  # 打印 revision 并对比内置兜底值

GitHub API 限流时（未登录 60 次/小时）可用 GITHUB_TOKEN 环境变量。
查询失败会以非 0 退出，构建脚本据此回退到内置 revision。
"""

import json
import os
import sys
import urllib.error
import urllib.request

REPO = "ACL4SSR/ACL4SSR"
API = f"https://api.github.com/repos/{REPO}/commits?per_page=1"
# 与 internal/scheme/revision.go 里的兜底值保持一致
FALLBACK = "6987aa2bf0350f39db4d9f54b84b0e92118a7f6e"


def latest_revision() -> str:
    req = urllib.request.Request(API, headers={"User-Agent": "hangbiao"})
    token = os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(req, timeout=20) as resp:
        data = json.load(resp)
    return data[0]["sha"]


def main() -> int:
    try:
        rev = latest_revision()
    except (urllib.error.URLError, KeyError, IndexError, TimeoutError) as exc:
        print(f"查询失败：{exc}", file=sys.stderr)
        return 1
    print(rev)
    if "--check" in sys.argv and rev != FALLBACK:
        print(f"内置兜底已过期：{FALLBACK} -> {rev}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
