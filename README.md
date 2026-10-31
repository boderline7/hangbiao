# 航标（hangbiao）

> 规则数据来源：[ACL4SSR/ACL4SSR](https://github.com/ACL4SSR/ACL4SSR)，按 commit 直连其仓库，不随本项目分发。
> 策略组交互参考：[pengchujin/tower](https://github.com/pengchujin/tower)（MIT），代码与界面独立重写。

Web 版代理配置生成器：Go 单二进制 + Docker，跑在 x86 / ARM 服务器上，生成 Clash(mihomo) 配置并对外提供订阅地址，也可导出 PassWall 的 Base64 节点订阅。

核心模型是**有序策略组**：组的位置就是规则优先级，导出时按组顺序摊平规则，最后补 `MATCH`。调整优先级只需拖拽，不用改模板，也不会被订阅刷新覆盖。

## 功能

- **订阅**：Clash YAML、Base64、明文 `ss / vmess / trojan / vless / hysteria2 / hysteria / tuic / anytls`，支持粘贴原文
- **分组**：拖拽排序，select / url-test / fallback / load-balance；成员支持 `$nodes` / `$groups` / `$regions` 占位，可隐藏
- **地区组**：按节点名国旗/关键字识别港、日、美、新、台、韩、英、德、法，自动生成选择组与隐藏的延迟优选组
- **规则**：13 种类型；规则集只在被引用时写入配置，规则数据走远程 URL
- **方案**：内置三套，分组页「同步 ACL4SSR」可拉取全部 33 份 ini，无需重建镜像
- **导出**：Clash、PassWall
- **开关（默认全开）**：IPv6、TUN + 加密 DNS、Tailscale 直连、本地 IP / 微力同步 / 常见 BT 下载直连、仅代理常见端口
- **订阅刷新**：按间隔定时刷新（默认 24 小时），也可一键刷新
- **节点筛选**：正则包含 / 不包含，对导出与分发统一生效

## 部署

```bash
docker run -d --name hangbiao \
  -p 8630:8630 \
  -v /opt/hangbiao:/data \
  --restart unless-stopped \
  ghcr.io/boderline7/hangbiao:latest
```

自动匹配 amd64 / arm64。

## 使用

1. **订阅** → 添加机场订阅
2. **分组** → 载入方案或新建组
3. 组内加站点或规则
4. 拖动调整组顺序（= 匹配优先级）
5. **导出** → 预览 → 复制订阅地址

客户端填 `http://<服务器IP>:8630/sub?target=auto`，需要固定格式时换成 `clash` / `passwall`。

## 数据

所有状态在容器内 `/data/state.json`，备份即复制该文件。

## 许可

MIT，见 `LICENSE`。
