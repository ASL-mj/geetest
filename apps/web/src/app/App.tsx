import {
  type LucideIcon,
  Activity,
  BarChart3,
  BookOpen,
  CheckCircle2,
  ChevronRight,
  CircleUserRound,
  Clipboard,
  Clock3,
  Code2,
  Copy,
  CreditCard,
  FileText,
  KeyRound,
  LayoutDashboard,
  ListFilter,
  LogOut,
  MoreHorizontal,
  Play,
  Plus,
  RefreshCw,
  Settings2,
  ShieldCheck,
  Terminal,
  X,
  Zap,
} from 'lucide-react';
import { type ReactNode, useState } from 'react';

import './App.css';

type PageId = 'dashboard' | 'keys' | 'debug' | 'usage' | 'calls' | 'docs' | 'account';
type StatusTone = 'success' | 'warning' | 'danger' | 'neutral' | 'info';

interface NavigationItem {
  id: PageId;
  label: string;
  icon: LucideIcon;
}

const navigation: NavigationItem[] = [
  { id: 'dashboard', label: '控制台概览', icon: LayoutDashboard },
  { id: 'keys', label: 'API Key 管理', icon: KeyRound },
  { id: 'debug', label: '在线接口调试', icon: Terminal },
  { id: 'usage', label: '用量统计', icon: BarChart3 },
  { id: 'calls', label: '调用日志', icon: FileText },
  { id: 'docs', label: '接口文档', icon: BookOpen },
  { id: 'account', label: '账号与服务', icon: CircleUserRound },
];

const pageTitles: Record<PageId, { title: string; description: string }> = {
  dashboard: { title: '控制台概览', description: '服务、额度和近期调用一目了然。' },
  keys: { title: 'API Key 管理', description: '为不同程序创建、管理和轮换调用密钥。' },
  debug: { title: '在线接口调试', description: '使用现有 Key 通过平台接口执行一次受控测试。' },
  usage: { title: '用量统计', description: '按时间和密钥观察调用量、成功率与额度变化。' },
  calls: { title: '调用日志', description: '查询每次请求的状态、耗时和配额变化。' },
  docs: { title: '接口文档', description: '接入地址、认证方式与标准响应说明。' },
  account: { title: '账号与服务', description: '查看 CDK 绑定、服务状态和安全设置。' },
};

const recentCalls = [
  { id: 'req_01J8V7BR4V1YAG2P5J0E', key: '生产环境', captcha: 'cap_6d18...c903', status: '成功', tone: 'success' as const, duration: '1.28 s', quota: '-1', time: '刚刚' },
  { id: 'req_01J8V79G13APJS8C7V4K', key: 'Worker A', captcha: 'cap_4f02...af1d', status: '成功', tone: 'success' as const, duration: '1.36 s', quota: '-1', time: '4 分钟前' },
  { id: 'req_01J8V726FGNWRNQ14M7C', key: '生产环境', captcha: 'cap_7ee3...d1b0', status: '失败已退回', tone: 'danger' as const, duration: '5.00 s', quota: '0', time: '13 分钟前' },
  { id: 'req_01J8V6V3AYKJ1QW28E4G', key: '回归测试', captcha: 'cap_229b...bf93', status: '成功', tone: 'success' as const, duration: '1.14 s', quota: '-1', time: '18 分钟前' },
];

const apiKeys = [
  { name: '生产环境', prefix: 'gtsk_live_7Qm...', suffix: '28f9', state: '已启用', tone: 'success' as const, calls: '3,842', lastUsed: '刚刚', created: '2026-08-21' },
  { name: 'Worker A', prefix: 'gtsk_live_4Lb...', suffix: 'c320', state: '已启用', tone: 'success' as const, calls: '1,209', lastUsed: '4 分钟前', created: '2026-08-28' },
  { name: '回归测试', prefix: 'gtsk_live_0Zr...', suffix: 'a7cd', state: '已禁用', tone: 'neutral' as const, calls: '421', lastUsed: '昨天 16:20', created: '2026-08-29' },
];

const usageBars = [42, 58, 47, 74, 64, 86, 71, 93, 82, 69, 88, 97];

function StatusPill({ children, tone = 'neutral' }: { children: string; tone?: StatusTone }) {
  return <span className={`status-pill status-pill--${tone}`}>{children}</span>;
}

function CopyButton({ text, label = '复制内容', compact = false }: { text: string; label?: string; compact?: boolean }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    await navigator.clipboard?.writeText(text);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  return (
    <button className={compact ? 'icon-button' : 'quiet-button'} onClick={() => void copy()} title={label} type="button">
      {copied ? <CheckCircle2 aria-hidden="true" size={16} /> : <Copy aria-hidden="true" size={16} />}
      {!compact && <span>{copied ? '已复制' : label}</span>}
    </button>
  );
}

function MetricCard({ label, value, hint, tone = 'default', icon: Icon }: { label: string; value: string; hint: string; tone?: 'default' | 'green' | 'orange' | 'blue'; icon: LucideIcon }) {
  return (
    <article className={`metric-card metric-card--${tone}`}>
      <div className="metric-card__heading">
        <span>{label}</span>
        <span className="metric-card__icon"><Icon aria-hidden="true" size={17} /></span>
      </div>
      <strong>{value}</strong>
      <p>{hint}</p>
    </article>
  );
}

function SectionHeading({ title, detail, action }: { title: string; detail?: string; action?: ReactNode }) {
  return (
    <div className="section-heading">
      <div>
        <h2>{title}</h2>
        {detail && <p>{detail}</p>}
      </div>
      {action}
    </div>
  );
}

function SampleLabel() {
  return (
    <span className="sample-label">
      <Clipboard aria-hidden="true" size={14} />
      界面示例数据
    </span>
  );
}

function Dashboard({ goTo }: { goTo: (page: PageId) => void }) {
  return (
    <>
      <div className="view-context"><SampleLabel /><span>接通账户与统计接口后将展示实时数据</span></div>
      <section className="metric-grid" aria-label="关键指标">
        <MetricCard label="服务状态" value="运行正常" hint="平台 API 可用" tone="green" icon={Activity} />
        <MetricCard label="剩余额度" value="7,846" hint="总额度 10,000 次" tone="blue" icon={Zap} />
        <MetricCard label="今日调用" value="126" hint="较昨日 +14.5%" tone="default" icon={BarChart3} />
        <MetricCard label="成功率" value="99.2%" hint="近 24 小时" tone="orange" icon={CheckCircle2} />
      </section>

      <section className="dashboard-grid dashboard-grid--top">
        <article className="surface quota-surface">
          <SectionHeading title="额度使用情况" detail="当前 CDK 服务额度" action={<StatusPill tone="success">有效</StatusPill>} />
          <div className="quota-number"><strong>7,846</strong><span>/ 10,000 次剩余</span></div>
          <div className="progress" aria-label="额度已使用 22%"><span /></div>
          <div className="quota-legend">
            <span><i className="legend-dot legend-dot--blue" />已使用 2,154</span>
            <span><i className="legend-dot legend-dot--muted" />剩余 7,846</span>
            <span>到期时间 <b>2026-12-31</b></span>
          </div>
          <button className="text-action" type="button" onClick={() => goTo('usage')}>查看用量统计 <ChevronRight aria-hidden="true" size={15} /></button>
        </article>

        <article className="surface endpoint-surface">
          <SectionHeading title="接入地址" detail="通过平台 API 调用解析服务" />
          <div className="endpoint-row">
            <code>POST /v1/geetest/solve</code>
            <CopyButton compact label="复制接口地址" text="https://api.your-domain.com/v1/geetest/solve" />
          </div>
          <pre aria-label="请求示例"><code>{`curl -X POST https://api.your-domain.com/v1/geetest/solve \\
  -H "Authorization: Bearer gtsk_live_..." \\
  -H "Idempotency-Key: request-unique-id"`}</code></pre>
          <div className="endpoint-footer"><Code2 aria-hidden="true" size={16} /><span>认证、额度和调用审计由平台统一处理</span></div>
        </article>
      </section>

      <section className="surface calls-surface">
        <SectionHeading
          title="最近调用"
          detail="按完成时间倒序展示"
          action={<button className="quiet-button" onClick={() => goTo('calls')} type="button">查看全部 <ChevronRight aria-hidden="true" size={16} /></button>}
        />
        <CallTable rows={recentCalls} compact />
      </section>
    </>
  );
}

function CallTable({ rows, compact = false }: { rows: typeof recentCalls; compact?: boolean }) {
  return (
    <div className="table-wrap">
      <table>
        <thead><tr><th>请求 ID</th><th>API Key</th><th>Captcha ID</th><th>状态</th><th>耗时</th><th>额度</th>{!compact && <th>完成时间</th>}</tr></thead>
        <tbody>
          {rows.map((call) => (
            <tr key={call.id}>
              <td><button className="mono-link" type="button" title="复制请求 ID">{call.id}</button></td>
              <td>{call.key}</td>
              <td><code>{call.captcha}</code></td>
              <td><StatusPill tone={call.tone}>{call.status}</StatusPill></td>
              <td className="mono">{call.duration}</td>
              <td className={call.quota === '0' ? 'quota-refund mono' : 'mono'}>{call.quota}</td>
              {!compact && <td>{call.time}</td>}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function KeysPage({ onCreate }: { onCreate: () => void }) {
  return (
    <>
      <div className="view-context"><SampleLabel /><span>Key 明文仅在创建成功时展示一次</span></div>
      <section className="surface key-summary">
        <div><p className="eyebrow">API KEY 限额</p><strong>2 <span>/ 5</span></strong><p>当前可继续创建 3 个 API Key，所有 Key 共享 CDK 额度。</p></div>
        <button className="primary-button" type="button" onClick={onCreate}><Plus aria-hidden="true" size={17} />新建 API Key</button>
      </section>
      <section className="surface">
        <SectionHeading title="我的 API Key" detail="已删除的 Key 不再显示，历史调用日志仍可追溯。" />
        <div className="table-wrap">
          <table>
            <thead><tr><th>名称</th><th>密钥标识</th><th>状态</th><th>调用次数</th><th>最后使用</th><th>创建时间</th><th aria-label="操作" /></tr></thead>
            <tbody>{apiKeys.map((key) => (
              <tr key={key.prefix}>
                <td><strong className="table-name">{key.name}</strong></td>
                <td><code>{key.prefix}{key.suffix}</code></td>
                <td><StatusPill tone={key.tone}>{key.state}</StatusPill></td>
                <td className="mono">{key.calls}</td>
                <td>{key.lastUsed}</td>
                <td>{key.created}</td>
                <td><button className="icon-button" title={`${key.name} 的更多操作`} type="button"><MoreHorizontal aria-hidden="true" size={18} /></button></td>
              </tr>
            ))}</tbody>
          </table>
        </div>
      </section>
    </>
  );
}

function DebugPage() {
  const [submitted, setSubmitted] = useState(false);
  return (
    <section className="debug-layout">
      <article className="surface form-surface">
        <SectionHeading title="请求参数" detail="调试请求与公开 API 使用相同的 Key、额度和审计规则。" />
        <label htmlFor="debug-key">API Key</label>
        <select defaultValue="production" id="debug-key"><option value="production">生产环境 · gtsk_live_7Qm...28f9</option><option value="worker">Worker A · gtsk_live_4Lb...c320</option></select>
        <label htmlFor="captcha-id">Captcha ID</label>
        <input id="captcha-id" placeholder="输入 Captcha ID" />
        <label htmlFor="risk-type">Risk Type</label>
        <select defaultValue="slide" id="risk-type"><option value="slide">slide</option></select>
        <button className="primary-button button-wide" type="button" onClick={() => setSubmitted(true)}><Play aria-hidden="true" size={16} />发送测试请求</button>
        {submitted && <p className="inline-info"><CheckCircle2 aria-hidden="true" size={16} />参数已校验。接通测试接口后将在此显示请求结果。</p>}
      </article>
      <article className="surface result-surface">
        <SectionHeading title="响应结果" detail="敏感字段仅对当前会话进行脱敏展示。" />
        <div className="empty-state"><Terminal aria-hidden="true" size={30} /><strong>等待测试请求</strong><p>填写 Captcha ID 后执行一次平台 API 调试。</p></div>
      </article>
    </section>
  );
}

function UsagePage() {
  return (
    <>
      <div className="view-context"><SampleLabel /><span>近 12 天的调用趋势示意</span></div>
      <section className="metric-grid">
        <MetricCard label="本周期调用" value="2,154" hint="2026-09-01 至今" icon={BarChart3} />
        <MetricCard label="成功调用" value="2,137" hint="成功率 99.2%" tone="green" icon={CheckCircle2} />
        <MetricCard label="失败已退回" value="17" hint="不消耗额度" tone="orange" icon={RefreshCw} />
        <MetricCard label="平均耗时" value="1.31 s" hint="P95 1.82 s" tone="blue" icon={Clock3} />
      </section>
      <section className="surface chart-surface">
        <SectionHeading title="调用趋势" detail="每日成功调用数" action={<button className="quiet-button" type="button"><ListFilter aria-hidden="true" size={16} />近 12 天</button>} />
        <div className="bar-chart" aria-label="调用趋势图">
          {usageBars.map((height, index) => <div className="bar-chart__column" key={index}><span style={{ height: `${height}%` }} /><small>{index % 2 === 0 ? `09/${String(index + 1).padStart(2, '0')}` : ''}</small></div>)}
        </div>
      </section>
      <section className="surface">
        <SectionHeading title="按 Key 聚合" detail="所有 Key 共用同一 CDK 配额池。" />
        <div className="table-wrap"><table><thead><tr><th>API Key</th><th>调用次数</th><th>成功率</th><th>消耗额度</th><th>最近调用</th></tr></thead><tbody>
          <tr><td>生产环境</td><td className="mono">1,484</td><td><StatusPill tone="success">99.5%</StatusPill></td><td className="mono">1,484</td><td>刚刚</td></tr>
          <tr><td>Worker A</td><td className="mono">670</td><td><StatusPill tone="success">98.7%</StatusPill></td><td className="mono">670</td><td>4 分钟前</td></tr>
        </tbody></table></div>
      </section>
    </>
  );
}

function CallsPage() {
  return (
    <>
      <div className="filter-bar"><button className="quiet-button" type="button"><ListFilter aria-hidden="true" size={16} />全部状态</button><button className="quiet-button" type="button">全部 API Key</button><button className="quiet-button" type="button"><Clock3 aria-hidden="true" size={16} />近 24 小时</button><button className="quiet-button filter-bar__end" type="button"><RefreshCw aria-hidden="true" size={16} />刷新</button></div>
      <section className="surface">
        <SectionHeading title="调用记录" detail="日志不会记录 API Key 明文、服务端凭据或完整敏感响应。" />
        <CallTable rows={recentCalls} />
        <div className="pagination"><span>共 126 条</span><div><button className="icon-button" title="上一页" type="button">‹</button><button className="page-button page-button--active" type="button">1</button><button className="page-button" type="button">2</button><button className="icon-button" title="下一页" type="button">›</button></div></div>
      </section>
    </>
  );
}

function DocsPage() {
  const snippet = `curl -X POST https://api.your-domain.com/v1/geetest/solve \\
  -H "Authorization: Bearer gtsk_live_your_key" \\
  -H "Content-Type: application/json" \\
  -H "Idempotency-Key: your-unique-request-id" \\
  -d '{"captcha_id":"captcha_id","risk_type":"slide"}'`;
  return (
    <section className="docs-layout">
      <article className="surface docs-toc"><p className="eyebrow">快速导航</p><a href="#quickstart">快速开始</a><a href="#authentication">认证方式</a><a href="#request">请求参数</a><a href="#response">响应与错误</a></article>
      <div className="docs-content">
        <article className="surface" id="quickstart"><SectionHeading title="快速开始" detail="使用平台发放的 API Key 调用统一的解析接口。" /><div className="doc-callout"><ShieldCheck aria-hidden="true" size={19} /><p>服务端保存底层解析服务凭据。浏览器与用户程序只使用平台 API Key。</p></div><h3 id="authentication">认证方式</h3><p>请求头使用 Bearer Token，并为每次请求提供唯一的幂等键。</p><div className="code-panel"><div><span>cURL</span><CopyButton compact label="复制示例" text={snippet} /></div><pre><code>{snippet}</code></pre></div></article>
        <article className="surface" id="request"><SectionHeading title="请求参数" /><div className="table-wrap"><table><thead><tr><th>字段</th><th>类型</th><th>必填</th><th>说明</th></tr></thead><tbody><tr><td><code>captcha_id</code></td><td>string</td><td>是</td><td>目标验证码标识</td></tr><tr><td><code>risk_type</code></td><td>string</td><td>是</td><td>V1 固定为 <code>slide</code></td></tr></tbody></table></div></article>
        <article className="surface" id="response"><SectionHeading title="响应与错误" detail="所有成功和失败响应都携带平台 request_id。" /><div className="error-grid"><div><StatusPill tone="success">200</StatusPill><p>解析成功，确认消耗一单位额度。</p></div><div><StatusPill tone="warning">422</StatusPill><p>请求参数错误，不消耗额度。</p></div><div><StatusPill tone="danger">502</StatusPill><p>下游异常，自动退回预扣额度。</p></div></div></article>
      </div>
    </section>
  );
}

function AccountPage() {
  return (
    <div className="account-grid">
      <section className="surface account-profile"><SectionHeading title="账号资料" detail="当前用户的基本信息与会话安全设置。" /><div className="profile-row"><span className="avatar">L</span><div><strong>linchen</strong><p>账号创建于 2026-08-21</p></div></div><div className="definition-list"><div><span>用户名</span><b>linchen</b></div><div><span>当前会话</span><b>macOS · Safari</b></div><div><span>最近登录</span><b>今天 10:42</b></div></div><button className="quiet-button" type="button"><Settings2 aria-hidden="true" size={16} />修改密码</button></section>
      <section className="surface"><SectionHeading title="服务授权" detail="CDK 状态决定关联 API Key 的有效调用状态。" /><div className="definition-list"><div><span>CDK 批次</span><b>2026-Q3-Standard</b></div><div><span>服务状态</span><StatusPill tone="success">有效</StatusPill></div><div><span>到期时间</span><b>2026-12-31 23:59 UTC</b></div><div><span>并发上限</span><b>5 个请求</b></div><div><span>每分钟上限</span><b>120 次请求</b></div></div></section>
      <section className="surface account-danger"><SectionHeading title="会话与安全" detail="退出操作会立即撤销当前浏览器会话。" /><button className="danger-button" type="button"><LogOut aria-hidden="true" size={16} />退出当前会话</button></section>
    </div>
  );
}

function CreateKeyDialog({ onClose }: { onClose: () => void }) {
  const [name, setName] = useState('');
  const [created, setCreated] = useState(false);
  const key = 'gtsk_live_7PmKX5nQ3Wv1Zc8T6aR4dH2s';
  return (
    <div className="dialog-backdrop" role="presentation">
      <section aria-labelledby="create-key-title" aria-modal="true" className="dialog" role="dialog">
        <div className="dialog__header"><div><h2 id="create-key-title">新建 API Key</h2><p>为使用场景设置便于识别的名称。</p></div><button className="icon-button" onClick={onClose} title="关闭弹窗" type="button"><X aria-hidden="true" size={18} /></button></div>
        {!created ? <><label htmlFor="key-name">Key 名称</label><input autoFocus id="key-name" onChange={(event) => setName(event.target.value)} placeholder="例如：生产环境" value={name} /><p className="field-hint">完整 Key 只会展示一次。请复制并保存到受保护的服务端环境变量中。</p><div className="dialog__actions"><button className="quiet-button" onClick={onClose} type="button">取消</button><button className="primary-button" disabled={!name.trim()} onClick={() => setCreated(true)} type="button"><Plus aria-hidden="true" size={16} />创建 Key</button></div></> : <><div className="success-message"><CheckCircle2 aria-hidden="true" size={21} /><div><strong>API Key 已创建</strong><p>关闭后无法再次查看完整密钥。</p></div></div><div className="secret-field"><code>{key}</code><CopyButton compact label="复制完整 Key" text={key} /></div><div className="dialog__actions"><button className="primary-button" onClick={onClose} type="button">我已安全保存</button></div></>}
      </section>
    </div>
  );
}

export function App() {
  const [activePage, setActivePage] = useState<PageId>('dashboard');
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const activeTitle = pageTitles[activePage];

  const renderPage = () => {
    switch (activePage) {
      case 'dashboard': return <Dashboard goTo={setActivePage} />;
      case 'keys': return <KeysPage onCreate={() => setShowCreateDialog(true)} />;
      case 'debug': return <DebugPage />;
      case 'usage': return <UsagePage />;
      case 'calls': return <CallsPage />;
      case 'docs': return <DocsPage />;
      case 'account': return <AccountPage />;
    }
  };

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand"><span className="brand__mark"><ShieldCheck aria-hidden="true" size={21} /></span><span>GeeTest<br /><b>Service Platform</b></span></div>
        <div className="workspace-label">用户控制台</div>
        <nav aria-label="主导航">
          {navigation.map((item) => {
            const Icon = item.icon;
            return <button className={`nav-item ${activePage === item.id ? 'nav-item--active' : ''}`} key={item.id} onClick={() => setActivePage(item.id)} type="button"><Icon aria-hidden="true" size={18} /><span>{item.label}</span></button>;
          })}
        </nav>
        <div className="sidebar__bottom"><div className="service-mini"><span><Activity aria-hidden="true" size={15} />服务运行正常</span><small>API 可用</small></div><button className="sidebar-help" type="button"><CreditCard aria-hidden="true" size={17} />服务与账单</button><button className="sidebar-help" type="button"><Settings2 aria-hidden="true" size={17} />平台设置</button></div>
      </aside>
      <div className="app-main">
        <header className="topbar"><div className="breadcrumb"><span>用户控制台</span><ChevronRight aria-hidden="true" size={15} /><b>{activeTitle.title}</b></div><div className="topbar__actions"><StatusPill tone="success">平台服务正常</StatusPill><button className="profile-button" onClick={() => setActivePage('account')} type="button"><span className="avatar avatar--small">L</span><span>linchen</span></button></div></header>
        <main className="content"><div className="page-title"><div><p className="eyebrow">GEE TEST SERVICE</p><h1>{activeTitle.title}</h1><p>{activeTitle.description}</p></div>{activePage === 'dashboard' && <button className="primary-button" onClick={() => setShowCreateDialog(true)} type="button"><Plus aria-hidden="true" size={17} />新建 API Key</button>}{activePage === 'keys' && <button className="primary-button page-title__button" onClick={() => setShowCreateDialog(true)} type="button"><Plus aria-hidden="true" size={17} />新建 API Key</button>}</div>{renderPage()}</main>
      </div>
      {showCreateDialog && <CreateKeyDialog onClose={() => setShowCreateDialog(false)} />}
    </div>
  );
}

export default App;
