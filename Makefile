.PHONY: tool fmt check tag push-tag release-patch release-minor gittag delcommit
#########################################
#### 标准发版 Makefile：release-patch / release-minor / push-tag / gittag / delcommit
#### 发布分两步：release-* 跑门禁并推 main + 打本地标签；确认远端 CI 通过后再 push-tag
########################################

LINT_TARGETS ?= ./...

# ---- 发版配置 ----
# 本文件是模板，刷新 harness 时会被整体覆盖：**不要在这里改配置值**。
# 各包自己的取值写进同目录的 Makefile.vars（入库，与团队共享），用 = 直接赋值即可——
# 它在下面这些 ?= 默认值之前加载，已定义的变量不会被默认值覆盖。
# 例：
#     # gosec：examples/ 里是示例连接串，不是真实凭据
#     GOSEC_EXCLUDE = G101
-include Makefile.vars

# BUMP               发版语义级别：patch（默认）/ minor
# RELEASE_REMOTE     发版推送的 remote 名
# COVERAGE_MIN       覆盖率下限（百分比整数），0 表示不检查。默认 0：发版流程不是发现
#                    覆盖率不足的合适时机，那时改代码为时已晚；要卡就设成具体数字
# REQUIRE_CHANGELOG  1 表示必须有可用的发布说明，否则拒绝发版。日常把变更写进
#                    ## [Unreleased]，发版时自动定版为 ## [新版本] - 今天并提取为标签说明。
#                    版本标题带不带 v 前缀都认，自动定版时跟随文件里已有条目的写法
# EXTRA_TEST_TARGET  发版前额外执行的 make 目标名（如集成测试），留空则跳过
# GOSEC_EXCLUDE      gosec 跳过的规则号，逗号分隔，留空则不跳过任何规则。
#                    每条排除都要在上面的注释里写明理由（为什么这条在本包里不成立），
#                    没有书面理由的排除等于把安全检查关掉
BUMP              ?= patch
RELEASE_REMOTE    ?= gtkit
COVERAGE_MIN      ?= 0
REQUIRE_CHANGELOG ?= 1
EXTRA_TEST_TARGET ?=
GOSEC_EXCLUDE     ?=

tool: ## 只读静态检查（不修改代码，格式化用 make fmt）
	@ echo "▶️ golangci-lint run"
	golangci-lint run $(LINT_TARGETS)
	@ echo "▶️ go fix -diff（现代写法门禁）"
	go fix -diff $(LINT_TARGETS)
	@ unformatted=$$(gofumpt -l .); \
	if [ -n "$$unformatted" ]; then \
	  echo "✗ 以下文件未按 gofumpt 格式化（运行 make fmt 修复）:"; echo "$$unformatted"; exit 1; \
	fi
	@ echo "✅ golangci-lint run"

fmt: ## 按 gofumpt 格式化代码（唯一允许写文件的格式化入口）
	gofumpt -l -w .

## govulncheck 检查漏洞 go install golang.org/x/vuln/cmd/govulncheck@latest
check:
	govulncheck ./...
	gosec $(if $(GOSEC_EXCLUDE),-exclude=$(GOSEC_EXCLUDE)) ./...
tag:
	@set -e; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "✗ 工作区不干净，发版前请先提交或清理："; git status --short; exit 1; \
	fi; \
	last=$$(git describe --tags --abbrev=0 2>/dev/null || true); \
	if [ -n "$$last" ]; then \
		echo "▶️ 空白检查 (git diff --check $$last..HEAD)"; \
		git diff --check "$$last..HEAD"; \
	fi; \
	echo "▶️ go mod tidy -diff"; go mod tidy -diff; \
	echo "▶️ go vet"; go vet ./...; \
	echo "▶️ go fix -diff（现代写法门禁）"; go fix -diff $(LINT_TARGETS); \
	echo "▶️ golangci-lint run"; golangci-lint run $(LINT_TARGETS); \
	echo "▶️ gofumpt 只读检查"; \
	unformatted=$$(gofumpt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "✗ 以下文件未按 gofumpt 格式化（运行 make fmt 修复）:"; echo "$$unformatted"; exit 1; \
	fi; \
	echo "▶️ 测试 (race)"; go test -race -count=1 -timeout=5m ./...; \
	if [ -n "$(EXTRA_TEST_TARGET)" ]; then \
		echo "▶️ 额外测试（$(EXTRA_TEST_TARGET)）"; $(MAKE) $(EXTRA_TEST_TARGET); \
	fi; \
	if [ "$(COVERAGE_MIN)" -gt 0 ] 2>/dev/null; then \
		echo "▶️ 覆盖率 ≥ $(COVERAGE_MIN)%"; \
		go test -coverprofile=coverage.out ./... >/dev/null; \
		cov=$$(go tool cover -func=coverage.out | awk '/^total:/ {gsub(/%/,"",$$3); print $$3}'); \
		rm -f coverage.out; \
		awk -v c="$$cov" -v m="$(COVERAGE_MIN)" 'BEGIN { if (c+0 < m+0) { printf "✗ 覆盖率 %.1f%% < %s%%\n", c+0, m; exit 1 } printf "✓ 覆盖率 %.1f%%\n", c+0 }'; \
	fi; \
	echo "▶️ benchmark"; go test -bench=. -benchmem -count=3 -run='^$$' ./... >/dev/null; \
	echo "▶️ govulncheck"; govulncheck ./...; \
	echo "▶️ gosec"; gosec -quiet $(if $(GOSEC_EXCLUDE),-exclude=$(GOSEC_EXCLUDE)) ./...; \
	current=$$(grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' version.go | head -n1 | tr -d 'v'); \
	if [ -z "$$current" ]; then echo "✗ version.go 中未找到版本号"; exit 1; fi; \
	maj=$$(echo $$current | cut -d. -f1); \
	min=$$(echo $$current | cut -d. -f2); \
	patch=$$(echo $$current | cut -d. -f3); \
	case "$(BUMP)" in \
	  patch) new="v$$maj.$$min.$$((patch+1))" ;; \
	  minor) new="v$$maj.$$((min+1)).0" ;; \
	  major) echo "✗ 本脚本不处理 MAJOR：只 bump tag 而不改 module path 是错误发布。破坏性变更留在当前 major 线以 MINOR 发布（make release-minor），并在 CHANGELOG 用 ⚠ 标注迁移说明"; exit 1 ;; \
	  *) echo "✗ BUMP 必须为 patch 或 minor（当前: $(BUMP)）"; exit 1 ;; \
	esac; \
	notes=""; changelog_bumped=0; today=$$(date +%Y-%m-%d); plain=$$(echo "$$new" | tr -d 'v'); \
	if grep -qE "^## \[v?$$plain\] - [0-9]{4}-[0-9]{2}-[0-9]{2}" CHANGELOG.md 2>/dev/null; then \
		notes=$$(awk -v h1="## [$$new] -" -v h2="## [$$plain] -" 'index($$0, h1) == 1 || index($$0, h2) == 1 { f = 1; next } f && /^## / { exit } f && /^### / { next } f { print }' CHANGELOG.md | sed -e '/./,$$!d'); \
	else \
		unreleased=$$(awk '/^## \[Unreleased\]/ { f = 1; next } f && /^## / { exit } f && /^### / { next } f { print }' CHANGELOG.md 2>/dev/null | sed -e '/./,$$!d'); \
		if [ -n "$$unreleased" ]; then \
			if grep -qE "^## \[v[0-9]" CHANGELOG.md; then head_ver="$$new"; else head_ver="$$plain"; fi; \
			awk -v line="## [$$head_ver] - $$today" '{ print } !d && /^## \[Unreleased\]/ { print ""; print line; d = 1 }' CHANGELOG.md > CHANGELOG.md.tmp && mv CHANGELOG.md.tmp CHANGELOG.md; \
			notes="$$unreleased"; changelog_bumped=1; \
			printf "▶️ CHANGELOG：[Unreleased] 已定版为 [%s] - %s\n" "$$head_ver" "$$today"; \
		elif [ "$(REQUIRE_CHANGELOG)" = "1" ]; then \
			echo "✗ 没有可用的发布说明：CHANGELOG.md 里既没有 ## [$$plain] - YYYY-MM-DD 条目，## [Unreleased] 下也没有内容"; \
			echo "  把本次变更写进 ## [Unreleased]，发版时会自动定版成 ## [$$plain] - $$today 并提取为标签说明"; exit 1; \
		fi; \
	fi; \
	printf "Bump (%s): v%s -> %s\n" "$(BUMP)" "$$current" "$$new"; \
	sed -E -i.bak 's/(const Version = ")([^"]+)(")/\1'"$$new"'\3/' version.go; \
	rm -f version.go.bak; \
	git add version.go; \
	if [ "$$changelog_bumped" = "1" ]; then git add CHANGELOG.md; fi; \
	git commit -m "chore(release): 发布 $$new"; \
	if [ -n "$$notes" ]; then \
		git tag -a "$$new" -m "$$(printf '版本 %s\n\n主要变更：\n%s\n' "$$new" "$$notes")"; \
	else \
		git tag -a "$$new" -m "$$(printf '版本 %s\n' "$$new")"; \
	fi; \
	git push $(RELEASE_REMOTE) HEAD; \
	printf "\n已推送 main，并在本地打好附注标签 %s。\n" "$$new"; \
	printf "标签**尚未**发布到远端：Go module proxy 一旦抓取标签就永久不可变，\n"; \
	printf "删除或覆盖都无法收回，因此等远端 CI 跑完再执行：\n\n    make push-tag\n\n"; \
	printf "push-tag 会自己核对这个 commit 的 CI 结论，未全绿会拒绝推送。\n\n"

push-tag: ## 发布第二步：远端 CI 通过后，把 HEAD 上的标签推送到远端
	@set -e; \
	tag=$$(git describe --tags --exact-match HEAD 2>/dev/null || true); \
	if [ -z "$$tag" ]; then \
		echo "✗ HEAD 上没有标签。先执行 make release-patch / make release-minor"; exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "✗ 工作区不干净，不应在此状态下发布标签："; git status --short; exit 1; \
	fi; \
	if git ls-remote --exit-code --tags $(RELEASE_REMOTE) "refs/tags/$$tag" >/dev/null 2>&1; then \
		printf "✓ %s 已在远端，无需重复推送\n" "$$tag"; exit 0; \
	fi; \
	if [ "$(SKIP_CI_CHECK)" != "1" ]; then \
		sha=$$(git rev-parse HEAD); \
		repo=$$(git remote get-url $(RELEASE_REMOTE) | sed -e 's|^git@github.com:||' -e 's|^https://github.com/||' -e 's|\.git$$||'); \
		printf "▶️ 核对 %s@%s 的 CI 结论\n" "$$repo" "$$sha"; \
		tok="$${GITHUB_TOKEN:-$$GH_TOKEN}"; \
		if [ -n "$$tok" ]; then auth="Authorization: Bearer $$tok"; else auth="X-No-Auth: 1"; fi; \
		body=$$(curl -fsSL -H "Accept: application/vnd.github+json" -H "$$auth" \
			"https://api.github.com/repos/$$repo/commits/$$sha/check-runs" 2>/dev/null || true); \
		if [ -z "$$body" ]; then \
			echo "✗ 查不到 CI 状态。可能原因：该 commit 未推送、网络不可达、仓库私有，"; \
			echo "  或未认证请求撞到 GitHub 的 60 次/小时限额（设 GITHUB_TOKEN 可提高限额）"; \
			echo "  已确认远端 CI 全绿时，可用 make push-tag SKIP_CI_CHECK=1 跳过本检查"; exit 1; \
		fi; \
		if ! printf '%s' "$$body" | python3 -c 'import json,sys; runs=json.load(sys.stdin).get("check_runs") or []; [print("   {} | {} | {}".format(r["name"], r["status"], r["conclusion"])) for r in runs]; sys.exit(0 if runs and all(r["status"] == "completed" and r["conclusion"] == "success" for r in runs) else 1)' 2>/dev/null; then \
			echo "✗ CI 未全绿、该 commit 尚无 CI 记录，或 API 响应无法解析——一律拒绝发布标签"; \
			echo "  已推送的标签会被 Go module proxy 永久缓存，不能等 CI 结果出来再补救"; exit 1; \
		fi; \
		echo "✓ CI 全绿"; \
	fi; \
	echo "▶️ 推送 $$tag 到 $(RELEASE_REMOTE)"; \
	git push $(RELEASE_REMOTE) "$$tag"; \
	printf "Published: %s\n" "$$tag"

release-patch: ## 发布 PATCH 版本（bug 修复 / 文档 / 内部重构）
	@$(MAKE) tag BUMP=patch

release-minor: ## 发布 MINOR 版本（向后兼容的新功能）
	@$(MAKE) tag BUMP=minor

gittag:
	git tag --sort=-version:refname | head -1

## 删除最近一次提交，但保留修改内容
delcommit:
	git reset --soft HEAD~1
