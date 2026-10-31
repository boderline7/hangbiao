package scheme

// ACL4SSRRevision 是 rule-provider URL 里固定的 ACL4SSR commit。
//
// 上游 tower 的 ACL4SSR_manifest.json 也按 revision 固定规则数据，本项目同样钉住
// 一个 commit（而不是 master），这样「载入方案」写出的订阅是可复现的。
//
// 这个变量在构建时被覆盖：
//   - GitHub Actions 每次构建都会查询 ACL4SSR/ACL4SSR 最新提交并注入，
//     所以发布出去的镜像用的是当天的最新规则数据；
//   - 本地构建可加 --build-arg ACL4SSR_REV=$(python scripts/acl4ssr_revision.py)。
//
// 未注入时保留最近一次核对过的 revision（2026-10-30 的 ACL4SSR HEAD）作为兜底。
var ACL4SSRRevision = "6987aa2bf0350f39db4d9f54b84b0e92118a7f6e"

// ACL4SSRDefaultRevision 是没被构建参数覆盖时的 revision，便于排查。
const ACL4SSRDefaultRevision = "6987aa2bf0350f39db4d9f54b84b0e92118a7f6e"

func acl4ssrBase() string {
	return "https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/" + Revision() + "/Clash/"
}
