<#
.SYNOPSIS
Go项目全量静态检查，完全对齐 Go Report Card 检测标准，缺失工具自动安装
#>
$ErrorActionPreference = "Continue"
$global:exitCode = 0
$target = "."

# 工具清单：名称、安装地址（移除addlicense，网页不需要它）
$tools = @(
    @{
        name = "gocyclo"
        path = "github.com/fzipp/gocyclo/cmd/gocyclo@latest"
    },
    @{
        name = "ineffassign"
        path = "github.com/gordonklaus/ineffassign@latest"
    },
    @{
        name = "misspell"
        path = "github.com/client9/misspell/cmd/misspell@latest"
    }
)

# 检查并自动安装工具
function Install-ToolIfMissing {
    param([string]$toolName, [string]$installPath)
    if (-not (Get-Command $toolName -ErrorAction SilentlyContinue)) {
        Write-Host "🔧 未检测到 $toolName，开始自动安装..." -ForegroundColor Yellow
        go install $installPath
        # 刷新PATH
        $env:Path += ";$env:GOPATH\bin;$env:GOROOT\bin"
        if (-not (Get-Command $toolName -ErrorAction SilentlyContinue)) {
            Write-Host "❌ $toolName 安装失败，请检查Go环境" -ForegroundColor Red
            exit 1
        }
        Write-Host "✅ $toolName 安装完成" -ForegroundColor Green
    }
}

function Check-Failed {
    param([string]$name)
    Write-Host "`n❌ $name 检测不通过！" -ForegroundColor Red
    $global:exitCode = 1
}

function Check-Pass {
    param([string]$name)
    Write-Host "✅ $name 100% 通过" -ForegroundColor Green
}

# 前置：安装第三方工具
Write-Host "===================== 检查并安装依赖工具 =====================" -ForegroundColor Cyan
foreach ($t in $tools) {
    Install-ToolIfMissing $t.name $t.path
}
Write-Host "===================== 开始全量代码检查 =====================" -ForegroundColor Cyan

# 1. gofmt 对齐平台：gofmt -s -l
Write-Host "`n[1/6] gofmt 代码格式化检查"
$fmtUnformatted = gofmt -s -l $target
if ($LASTEXITCODE -ne 0 -or $fmtUnformatted) {
    Write-Host "未格式化文件列表：`n$fmtUnformatted"
    Check-Failed "gofmt"
} else {
    Check-Pass "gofmt"
}

# 2. go vet
Write-Host "`n[2/6] go vet 静态语法检查"
go vet $target/...
if ($LASTEXITCODE -ne 0) {
    Check-Failed "go_vet"
} else {
    Check-Pass "go_vet"
}

# 3. gocyclo 对齐平台阈值15，过滤测试/厂商目录
Write-Host "`n[3/6] gocyclo 圈复杂度检查(阈值15)"
$cycloOver = gocyclo -over 15 -ignore "_test|vendor|testdata" $target
if ($LASTEXITCODE -ne 0 -or $cycloOver) {
    Write-Host "高复杂度函数列表：`n$cycloOver"
    Check-Failed "gocyclo"
} else {
    Check-Pass "gocyclo"
}

# 4. ineffassign
Write-Host "`n[4/6] ineffassign 无效赋值检查"
$ineffRes = ineffassign $target
if ($LASTEXITCODE -ne 0 -or $ineffRes) {
    Write-Host "无效赋值：`n$ineffRes"
    Check-Failed "ineffassign"
} else {
    Check-Pass "ineffassign"
}

# 5. license 完全对齐网页规则：仅校验根目录 LICENSE 文件是否存在
Write-Host "`n[5/6] license 版权文件检查"
if (-not (Test-Path "./LICENSE")) {
    Write-Host "项目根目录缺失 LICENSE 文件"
    Check-Failed "license"
} else {
    Check-Pass "license"
}

# 6. misspell 拼写检查
Write-Host "`n[6/6] misspell 英文拼写检查"
$spellErr = misspell $target
if ($LASTEXITCODE -ne 0 -or $spellErr) {
    Write-Host "拼写错误：`n$spellErr"
    Check-Failed "misspell"
} else {
    Check-Pass "misspell"
}

Write-Host "`n===================== 检查汇总 =====================" -ForegroundColor Cyan
if ($global:exitCode -ne 0) {
    Write-Host "❌ 存在检测失败项，请修复后重新运行" -ForegroundColor Red
    exit $global:exitCode
}
Write-Host "🎉 全部检查 100% 通过！A+ Excellent!" -ForegroundColor Green
exit 0
