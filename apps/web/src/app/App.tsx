import {
  type LucideIcon,
  Activity,
  BarChart3,
  BookOpen,
  CheckCircle2,
  ChevronRight,
  CircleUserRound,
  Copy,
  FileText,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Play,
  Plus,
  RefreshCw,
  ShieldCheck,
  Terminal,
  X,
  Zap,
} from 'lucide-react';
import { type ReactNode, useCallback, useEffect, useRef, useState } from 'react';

import './App.css';
import { AdminPage } from './AdminPage';
import { AuthPage, PublicDocs, PublicHome } from './PublicPages';
import { ApiError, api, getDocsBaseURL, type APIKey, type CallRecord, type CDKSummary, type UsageReport } from '../lib/api';
import { consolePath, navigate, useRoute, type ConsolePage } from '../lib/router';
import { callStatusMeta, cdkStatusMeta, formatCount, formatTime, keyStatusMeta, type StatusTone } from '../lib/status';

type PageId = 'dashboard' | 'keys' | 'debug' | 'usage' | 'calls' | 'docs' | 'account';

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
];

const pageTitles: Record<PageId, { title: string; description: string }> = {
  dashboard: { title: '控制台概览', description: '服务、额度和近期调用一目了然。' },
  keys: { title: 'API Key 管理', description: '为不同程序创建、管理和轮换调用密钥。' },
  debug: { title: '在线接口调试', description: '使用现有 Key 通过平台接口执行一次受控测试。' },
  usage: { title: '用量统计', description: '观察调用量、成功率与额度变化。' },
  calls: { title: '调用日志', description: '查询每次请求的状态、耗时和配额变化。' },
  docs: { title: '接口文档', description: '接入地址、认证方式与标准响应说明。' },
  account: { title: '账号与服务', description: '查看 CDK 绑定、服务状态和安全设置。' },
};

function StatusPill({ children, tone = 'neutral' }: { children: string; tone?: StatusTone }) {
  return <span className={`status-pill status-pill--${tone}`}>{children}</span>;
}

function CopyButton({ text, label = '复制内容', compact = false }: { text: string; label?: string; compact?: boolean }) {
  const [copied, setCopied] = useState(false);

  const copy = () => {
    navigator.clipboard?.writeText(text).catch(() => {});
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

function LoadError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="empty-state" role="alert">
      <RefreshCw aria-hidden="true" size={26} />
      <strong>数据加载失败</strong>
      <p>{message}</p>
      <button className="quiet-button" onClick={onRetry} type="button">重试</button>
    </div>
  );
}

// ---------- dashboard ----------

function CallTable({ rows, onOpen }: { rows: CallRecord[]; onOpen?: (requestId: string) => void }) {
  return (
    <div className="table-wrap">
      <table>
        <thead><tr><th>请求 ID</th><th>API Key</th><th>Captcha ID</th><th>状态</th><th>耗时</th><th>额度</th><th>时间</th></tr></thead>
        <tbody>
          {rows.map((call) => {
            const meta = callStatusMeta(call.status);
            return (
              <tr key={call.request_id}>
                <td><button className="mono-link" onClick={() => onOpen?.(call.request_id)} title="查看详情" type="button">{call.request_id.slice(0, 18)}…</button></td>
                <td>{call.api_key_name}</td>
                <td><code>{call.captcha_id.slice(0, 14)}</code></td>
                <td><StatusPill tone={meta.tone}>{meta.label}</StatusPill></td>
                <td className="mono">{call.duration_ms != null ? `${(call.duration_ms / 1000).toFixed(2)} s` : '—'}</td>
                <td className={call.quota_refunded ? 'quota-refund mono' : 'mono'}>{call.quota_refunded ? '退回' : '-1'}</td>
                <td>{formatTime(call.accepted_at)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function Dashboard({ goTo }: { goTo: (page: PageId) => void }) {
  const [usage, setUsage] = useState<UsageReport | null>(null);
  const [calls, setCalls] = useState<CallRecord[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [baseURL, setBaseURL] = useState(window.location.origin);

  useEffect(() => {
    void getDocsBaseURL().then(setBaseURL);
  }, []);

  useEffect(() => {
    let alive = true;
    setError(null);
    void (async () => {
      try {
        const [usageEnvelope, callsEnvelope] = await Promise.all([api.usage(), api.listCalls(undefined, 5)]);
        if (!alive) return;
        setUsage(usageEnvelope.data ?? null);
        setCalls(callsEnvelope.data?.items ?? []);
      } catch (err) {
        if (alive) setError(err instanceof ApiError ? err.message : '网络错误。');
      }
    })();
    return () => { alive = false; };
  }, [reloadKey]);

  const retry = () => setReloadKey((value) => value + 1);

  if (error) return <LoadError message={error} onRetry={retry} />;
  if (!usage) return <div className="empty-state"><RefreshCw aria-hidden="true" size={26} /><strong>正在加载…</strong></div>;

  const used = usage.quota_used;
  const total = usage.quota_total;
  const percent = total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0;

  return (
    <>
      <section className="metric-grid" aria-label="关键指标">
        <MetricCard label="服务状态" value="运行正常" hint="平台 API 可用" tone="green" icon={Activity} />
        <MetricCard label="剩余额度" value={formatCount(usage.quota_remaining)} hint={`总额度 ${formatCount(usage.quota_total)} 次`} tone="blue" icon={Zap} />
        <MetricCard label="今日调用" value={formatCount(usage.calls_today)} hint={`累计 ${formatCount(usage.calls_total)} 次`} tone="default" icon={BarChart3} />
        <MetricCard label="成功率" value={`${(usage.success_rate * 100).toFixed(1)}%`} hint="已结算调用" tone="orange" icon={CheckCircle2} />
      </section>

      <section className="dashboard-grid dashboard-grid--top">
        <article className="surface quota-surface">
          <SectionHeading title="额度使用情况" detail="当前 CDK 服务额度" action={<StatusPill tone="success">{cdkStatusMeta('ACTIVE').label}</StatusPill>} />
          <div className="quota-number"><strong>{formatCount(usage.quota_remaining)}</strong><span>/ {formatCount(total)} 次剩余</span></div>
          <div className="progress" aria-label={`额度已使用 ${percent}%`}><span style={{ width: `${percent}%` }} /></div>
          <div className="quota-legend">
            <span><i className="legend-dot legend-dot--blue" />已使用 {formatCount(used)}</span>
            <span><i className="legend-dot legend-dot--muted" />剩余 {formatCount(usage.quota_remaining)}</span>
            {usage.quota_reserved > 0 && <span>在途 {formatCount(usage.quota_reserved)}</span>}
          </div>
          <button className="text-action" type="button" onClick={() => goTo('usage')}>查看用量统计 <ChevronRight aria-hidden="true" size={15} /></button>
        </article>

        <article className="surface endpoint-surface">
          <SectionHeading title="接入地址" detail="通过平台 API 调用解析服务" />
          <div className="endpoint-row">
            <code>POST {baseURL}/v1/captcha/solve</code>
            <CopyButton compact label="复制接口地址" text={`${baseURL}/v1/captcha/solve`} />
          </div>
          <pre aria-label="请求示例"><code>{`curl -X POST ${baseURL}/v1/captcha/solve \\
  -H "Authorization: Bearer cf_live_..." \\
  -H "Idempotency-Key: request-unique-id"`}</code></pre>
          <div className="endpoint-footer"><Terminal aria-hidden="true" size={16} /><span>认证、额度和调用审计由平台统一处理</span></div>
        </article>
      </section>

      <section className="surface calls-surface">
        <SectionHeading
          title="最近调用"
          detail="按完成时间倒序展示"
          action={<button className="quiet-button" onClick={() => goTo('calls')} type="button">查看全部 <ChevronRight aria-hidden="true" size={16} /></button>}
        />
        {calls.length === 0
          ? <div className="empty-state"><Terminal aria-hidden="true" size={26} /><strong>还没有调用记录</strong><p>用平台 API Key 发起第一次解析后，这里会展示调用状态。</p></div>
          : <CallTable rows={calls} />}
      </section>
    </>
  );
}

// ---------- keys ----------

const keyLimit = 5;

function KeysPage() {
  const [keys, setKeys] = useState<APIKey[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [editTarget, setEditTarget] = useState<APIKey | null>(null);
  const [revealed, setRevealed] = useState<{ secret?: string; error?: string } | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  const refresh = () => setReloadKey((value) => value + 1);

  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const envelope = await api.listKeys();
        if (alive) setKeys(envelope.data?.items ?? []);
      } catch (err) {
        if (alive) setError(err instanceof ApiError ? err.message : '网络错误。');
      }
    })();
    return () => { alive = false; };
  }, [reloadKey]);

  const toggle = async (key: APIKey) => {
    setBusyKey(key.id);
    setNotice(null);
    try {
      await api.updateKey(key.id, { status: key.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE' });
      refresh();
    } catch (err) {
      setNotice(err instanceof ApiError ? err.message : '操作失败，请稍后重试。');
    } finally {
      setBusyKey(null);
    }
  };
  const remove = async (key: APIKey) => {
    if (!window.confirm(`删除 API Key「${key.name}」？删除后不可恢复。`)) return;
    setBusyKey(key.id);
    setNotice(null);
    try {
      await api.deleteKey(key.id);
      refresh();
    } catch (err) {
      setNotice(err instanceof ApiError ? err.message : '删除失败，请稍后重试。');
    } finally {
      setBusyKey(null);
    }
  };
  const copySecret = async (key: APIKey) => {
    try {
      const envelope = await api.revealKeySecret(key.id);
      const secret = envelope.data?.secret ?? '';
      if (!secret) throw new ApiError(422, 'KEY_SECRET_UNAVAILABLE', '该 Key 缺少可恢复的密文。', '');
      // Show the dialog first; the clipboard write is best-effort because a
      // pending permission prompt must never block the UI.
      setRevealed({ secret });
      navigator.clipboard?.writeText(secret).catch(() => {});
    } catch (err) {
      setRevealed({ error: err instanceof ApiError ? err.message : '无法获取密钥明文。' });
    }
  };

  if (error) return <LoadError message={error} onRetry={refresh} />;
  const activeCount = keys?.filter((key) => key.status !== 'DELETED').length ?? 0;

  return (
    <>
      <section className="surface">
        <SectionHeading
          title="我的 API Key"
          detail="已删除的 Key 不再显示，历史调用日志仍可追溯。"
          action={<button className="primary-button" onClick={() => setShowCreate(true)} type="button"><Plus aria-hidden="true" size={17} />新建 API Key</button>}
        />
        <p className="key-count-hint">共 <b>{activeCount}</b> / {keyLimit} 个（所有 Key 共享 CDK 额度池）</p>
        {notice && <p className="key-notice" role="alert">{notice}</p>}
        {keys === null
          ? <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在加载…</strong></div>
          : keys.length === 0
            ? <div className="empty-state"><KeyRound aria-hidden="true" size={26} /><strong>还没有 API Key</strong><p>创建第一个 Key 并保存好明文即可开始调用。</p></div>
            : (
              <div className="table-wrap">
                <table>
                  <thead><tr><th>名称</th><th>密钥标识</th><th>状态</th><th>限额</th><th>IP 白名单</th><th>调用次数</th><th>最后使用</th><th aria-label="操作" /></tr></thead>
                  <tbody>{keys.map((key) => {
                    const meta = keyStatusMeta(key.status);
                    return (
                      <tr key={key.id}>
                        <td><strong className="table-name">{key.name}</strong></td>
                        <td><code>{key.prefix}…{key.last4}</code></td>
                        <td><StatusPill tone={meta.tone}>{meta.label}</StatusPill></td>
                        <td className="mono">{key.quota_limit != null ? formatCount(key.quota_limit) : '不限'}</td>
                        <td>{key.allowed_ips ? <code className="mono">{key.allowed_ips}</code> : '不限'}</td>
                        <td className="mono">{formatCount(key.total_calls)}</td>
                        <td>{formatTime(key.last_used_at)}</td>
                        <td>
                          {key.status !== 'DELETED' && (
                            <div className="row-actions">
                              <button className="action-link" disabled={busyKey === key.id} onClick={() => void copySecret(key)} type="button">复制</button>
                              <button className="action-link" disabled={busyKey === key.id} onClick={() => setEditTarget(key)} type="button">编辑</button>
                              <button className="action-link" disabled={busyKey === key.id} onClick={() => void toggle(key)} type="button">{key.status === 'ACTIVE' ? '禁用' : '启用'}</button>
                              <button className="action-link action-link--danger" disabled={busyKey === key.id} onClick={() => void remove(key)} type="button">删除</button>
                            </div>
                          )}
                        </td>
                      </tr>
                    );
                  })}</tbody>
                </table>
              </div>
            )}
      </section>
      {showCreate && <CreateKeyDialog onClose={() => setShowCreate(false)} onCreated={refresh} />}
      {editTarget && <EditKeyDialog key={editTarget.id} apiKey={editTarget} onClose={() => setEditTarget(null)} onSaved={refresh} />}
      {revealed && (
        <div className="dialog-backdrop" role="presentation">
          <section aria-labelledby="reveal-title" aria-modal="true" className="dialog" role="dialog">
            <div className="dialog__header"><div><h2 id="reveal-title">{revealed.secret ? '密钥明文' : '无法显示明文'}</h2><p>{revealed.secret ? '已复制到剪贴板；请勿泄露给他人。' : '该 Key 创建于历史版本，没有可恢复的密文；请新建 Key 并使用新密钥。'}</p></div><button className="icon-button" onClick={() => setRevealed(null)} title="关闭" type="button"><X aria-hidden="true" size={18} /></button></div>
            {revealed.secret && <div className="secret-field"><code>{revealed.secret}</code><CopyButton compact label="复制" text={revealed.secret} /></div>}
            <div className="dialog__actions"><button className="primary-button" onClick={() => setRevealed(null)} type="button">完成</button></div>
          </section>
        </div>
      )}
    </>
  );
}

function EditKeyDialog({ apiKey, onClose, onSaved }: { apiKey: APIKey; onClose: () => void; onSaved: () => void }) {
  useEscapeToClose(onClose);
  const [name, setName] = useState(apiKey.name);
  const [quotaLimit, setQuotaLimit] = useState(apiKey.quota_limit?.toString() ?? '');
  const [allowedIPs, setAllowedIPs] = useState(apiKey.allowed_ips ?? '');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const save = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const trimmedQuota = quotaLimit.trim();
      await api.updateKey(apiKey.id, {
        name: name.trim(),
        quota_limit: trimmedQuota === '' ? 0 : Number(trimmedQuota),
        allowed_ips: allowedIPs.trim(),
      });
      onSaved();
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '保存失败，请稍后重试。');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="dialog-backdrop" role="presentation">
      <section aria-labelledby="edit-key-title" aria-modal="true" className="dialog" role="dialog">
        <div className="dialog__header"><div><h2 id="edit-key-title">编辑 API Key</h2><p>{apiKey.prefix}…{apiKey.last4}</p></div><button className="icon-button" onClick={onClose} title="关闭弹窗" type="button"><X aria-hidden="true" size={18} /></button></div>
        <label htmlFor="edit-name">名称</label>
        <input id="edit-name" onChange={(event) => setName(event.target.value)} value={name} />
        <label htmlFor="edit-quota">限额次数（留空或 0 表示不限）</label>
        <input id="edit-quota" inputMode="numeric" min={0} onChange={(event) => setQuotaLimit(event.target.value)} placeholder="例如：1000" type="number" value={quotaLimit} />
        <p className="field-hint">达到限额后该 Key 将被拒绝调用（402），不影响其他 Key。</p>
        <label htmlFor="edit-ips">IP 白名单（IP 或 CIDR，逗号分隔；留空表示不限）</label>
        <textarea id="edit-ips" onChange={(event) => setAllowedIPs(event.target.value)} placeholder="例如：203.0.113.7, 198.51.100.0/24" rows={2} value={allowedIPs} />
        <p className="field-hint">启用后，只有白名单内来源 IP 可以使用该 Key 调用解析接口。</p>
        {error && <p className="field-hint" role="alert">{error}</p>}
        <div className="dialog__actions">
          <button className="quiet-button" onClick={onClose} type="button">取消</button>
          <button className="primary-button" disabled={!name.trim() || submitting} onClick={() => void save()} type="button">{submitting ? '保存中…' : '保存'}</button>
        </div>
      </section>
    </div>
  );
}

function CreateKeyDialog({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  useEscapeToClose(onClose);
  const [name, setName] = useState('');
  const [secret, setSecret] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const create = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const envelope = await api.createKey(name.trim());
      setSecret(envelope.data?.secret ?? null);
      onCreated();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '创建失败，请稍后重试。');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="dialog-backdrop" role="presentation">
      <section aria-labelledby="create-key-title" aria-modal="true" className="dialog" role="dialog">
        <div className="dialog__header"><div><h2 id="create-key-title">新建 API Key</h2><p>为使用场景设置便于识别的名称。</p></div><button className="icon-button" onClick={onClose} title="关闭弹窗" type="button"><X aria-hidden="true" size={18} /></button></div>
        {!secret ? <>
          <label htmlFor="key-name">Key 名称</label>
          <input autoFocus id="key-name" onChange={(event) => setName(event.target.value)} placeholder="例如：生产环境" value={name} />
          <p className="field-hint">完整 Key 只会展示一次。请复制并保存到受保护的服务端环境变量中。</p>
          {error && <p className="field-hint" role="alert">{error}</p>}
          <div className="dialog__actions">
            <button className="quiet-button" onClick={onClose} type="button">取消</button>
            <button className="primary-button" disabled={!name.trim() || submitting} onClick={() => void create()} type="button"><Plus aria-hidden="true" size={16} />创建 Key</button>
          </div>
        </> : <>
          <div className="success-message"><CheckCircle2 aria-hidden="true" size={21} /><div><strong>API Key 已创建</strong><p>关闭后无法再次查看完整密钥。</p></div></div>
          <div className="secret-field"><code>{secret}</code><CopyButton compact label="复制完整 Key" text={secret} /></div>
          <div className="dialog__actions"><button className="primary-button" onClick={onClose} type="button">我已安全保存</button></div>
        </>}
      </section>
    </div>
  );
}

// ---------- debug playground ----------

function DebugPage() {
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [keyId, setKeyId] = useState('');
  const [captchaId, setCaptchaId] = useState('');
  const [result, setResult] = useState<Record<string, unknown> | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    void (async () => {
      try {
        const envelope = await api.listKeys();
        const items = (envelope.data?.items ?? []).filter((key) => key.status === 'ACTIVE');
        setKeys(items);
        setKeyId(items[0]?.id ?? '');
      } catch {
        setKeys([]);
      }
    })();
  }, []);

  const submit = async () => {
    setSubmitting(true);
    setError(null);
    setResult(null);
    try {
      const envelope = await api.consoleSolve(keyId, captchaId.trim());
      setResult((envelope.data ?? {}) as Record<string, unknown>);
    } catch (err) {
      setError(err instanceof ApiError ? `${err.message}（${err.code}）` : '请求失败，请稍后重试。');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <section className="debug-layout">
      <article className="surface form-surface">
        <SectionHeading title="请求参数" detail="调试请求与公开 API 使用相同的 Key、额度和审计规则。" />
        <label htmlFor="debug-key">API Key</label>
        <select id="debug-key" onChange={(event) => setKeyId(event.target.value)} value={keyId}>
          {keys.length === 0 && <option value="">（暂无可用 Key）</option>}
          {keys.map((key) => <option key={key.id} value={key.id}>{key.name} · {key.prefix}…{key.last4}</option>)}
        </select>
        <label htmlFor="captcha-id">Captcha ID</label>
        <input id="captcha-id" onChange={(event) => setCaptchaId(event.target.value)} placeholder="输入 Captcha ID" value={captchaId} />
        <p className="field-hint">risk_type 固定为 slide；调用会按正常规则预扣额度。</p>
        <button className="primary-button button-wide" disabled={!keyId || !captchaId.trim() || submitting} onClick={() => void submit()} type="button"><Play aria-hidden="true" size={16} />{submitting ? '请求中…' : '发送测试请求'}</button>
        {error && <p className="field-hint" role="alert">{error}</p>}
      </article>
      <article className="surface result-surface">
        <SectionHeading title="响应结果" detail="成功返回解析字段；失败返回平台错误码。" />
        {result ? (
          <pre aria-label="解析结果"><code>{JSON.stringify(result, null, 2)}</code></pre>
        ) : (
          <div className="empty-state"><Terminal aria-hidden="true" size={30} /><strong>等待测试请求</strong><p>填写 Captcha ID 后执行一次平台 API 调试。</p></div>
        )}
      </article>
    </section>
  );
}

// ---------- usage ----------

function UsagePage() {
  const [usage, setUsage] = useState<UsageReport | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const envelope = await api.usage();
        if (alive) setUsage(envelope.data ?? null);
      } catch (err) {
        if (alive) setError(err instanceof ApiError ? err.message : '网络错误。');
      }
    })();
    return () => { alive = false; };
  }, [reloadKey]);

  if (error) return <LoadError message={error} onRetry={() => setReloadKey((value) => value + 1)} />;
  if (!usage) return <div className="empty-state"><RefreshCw aria-hidden="true" size={26} /><strong>正在加载…</strong></div>;

  return (
    <>
      <section className="metric-grid">
        <MetricCard label="累计调用" value={formatCount(usage.calls_total)} hint={`今日 ${formatCount(usage.calls_today)} 次`} icon={BarChart3} />
        <MetricCard label="成功调用" value={formatCount(usage.success_total)} hint={`成功率 ${(usage.success_rate * 100).toFixed(1)}%`} tone="green" icon={CheckCircle2} />
        <MetricCard label="失败已退回" value={formatCount(usage.failed_total)} hint="不消耗额度" tone="orange" icon={RefreshCw} />
        <MetricCard label="被拒绝" value={formatCount(usage.rejected_total)} hint="限流/并发/额度不足" tone="blue" icon={Zap} />
      </section>
      <section className="surface chart-surface">
        <SectionHeading title="额度结算" detail="预扣、确认与退回的额度变化" />
        <div className="definition-list">
          <div><span>总额度</span><b>{formatCount(usage.quota_total)}</b></div>
          <div><span>已使用</span><b>{formatCount(usage.quota_used)}</b></div>
          <div><span>在途预扣</span><b>{formatCount(usage.quota_reserved)}</b></div>
          <div><span>剩余可用</span><b>{formatCount(usage.quota_remaining)}</b></div>
        </div>
      </section>
    </>
  );
}

// ---------- calls ----------

function CallsPage() {
  const [pages, setPages] = useState<CallRecord[][]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [detail, setDetail] = useState<CallRecord | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  const loadingRef = useRef(false);
  const load = useCallback(async (after?: string) => {
    if (loadingRef.current) return;
    loadingRef.current = true;
    setError(null);
    try {
      const envelope = await api.listCalls(after);
      // The first page replaces (StrictMode/mount double-runs must not
      // duplicate rows); cursor loads append.
      if (after) {
        setPages((previous) => [...previous, envelope.data?.items ?? []]);
      } else {
        setPages([envelope.data?.items ?? []]);
      }
      setCursor(envelope.data?.next_cursor ?? null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '网络错误。');
    } finally {
      loadingRef.current = false;
    }
  }, []);

  useEffect(() => { void load(undefined); }, [load, reloadKey]);

  useEscapeToClose(() => setDetail(null));

  const openDetail = async (requestId: string) => {
    try {
      const envelope = await api.getCall(requestId);
      setDetail(envelope.data ?? null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '加载详情失败。');
    }
  };

  const flat = pages.flat();
  if (error && flat.length === 0) return <LoadError message={error} onRetry={() => { setPages([]); setReloadKey((value) => value + 1); }} />;

  return (
    <>
      <section className="surface">
        <SectionHeading title="调用记录" detail="日志不会记录 API Key 明文、服务端凭据或完整敏感响应。" />
        {flat.length === 0
          ? <div className="empty-state"><FileText aria-hidden="true" size={26} /><strong>暂无调用记录</strong></div>
          : <CallTable rows={flat} onOpen={(id) => void openDetail(id)} />}
        {error && flat.length > 0 && <p className="field-hint" role="alert">{error}</p>}
        {cursor && <div className="pagination"><button className="quiet-button" onClick={() => void load(cursor)} type="button">加载更多</button></div>}
        {detail && (
          <div className="dialog-backdrop" role="presentation" onClick={() => setDetail(null)}>
            <section aria-labelledby="call-detail-title" aria-modal="true" className="dialog" onClick={(event) => event.stopPropagation()} role="dialog">
              <div className="dialog__header"><div><h2 id="call-detail-title">调用详情</h2><p>{detail.request_id}</p></div><button className="icon-button" onClick={() => setDetail(null)} title="关闭" type="button"><X aria-hidden="true" size={18} /></button></div>
              <div className="definition-list">
                <div><span>状态</span><b>{callStatusMeta(detail.status).label}</b></div>
                <div><span>HTTP 状态码</span><b>{detail.http_status}</b></div>
                <div><span>Captcha ID</span><b>{detail.captcha_id}</b></div>
                <div><span>API Key</span><b>{detail.api_key_name}</b></div>
                <div><span>接受时间</span><b>{formatTime(detail.accepted_at)}</b></div>
                <div><span>完成时间</span><b>{formatTime(detail.completed_at)}</b></div>
                <div><span>耗时</span><b>{detail.duration_ms != null ? `${detail.duration_ms} ms` : '—'}</b></div>
                <div><span>错误码</span><b>{detail.error_code ?? '—'}</b></div>
                <div><span>脱敏来源</span><b>{detail.client_ip}</b></div>
              </div>
              <div className="dialog__actions"><CopyButton text={detail.request_id} label="复制请求 ID" /></div>
            </section>
          </div>
        )}
      </section>
    </>
  );
}

// ---------- account ----------

function AccountPage() {
  const [cdk, setCdk] = useState<CDKSummary | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [docsURL, setDocsURL] = useState(window.location.origin);

  useEffect(() => {
    void getDocsBaseURL().then(setDocsURL);
  }, []);

  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const envelope = await api.account();
        if (alive) setCdk(envelope.data?.cdk ?? null);
      } catch (err) {
        if (alive) setError(err instanceof ApiError ? err.message : '网络错误。');
      }
    })();
    return () => { alive = false; };
  }, []);

  if (error) return <LoadError message={error} onRetry={() => window.location.reload()} />;

  return (
    <div className="account-grid">
      <section className="surface account-profile">
        <SectionHeading title="身份资料" detail="CDK 是当前服务身份，浏览器仅保留短期会话。" />
        <div className="profile-row"><span className="avatar">C</span><div><strong>CDK 用户</strong><p>激活时间 {formatTime(cdk?.activated_at ?? null)}</p></div></div>
        <div className="definition-list">
          <div><span>身份方式</span><b>CDK 凭证</b></div>
          <div><span>CDK 前缀</span><b className="mono">{cdk?.code_prefix ?? '—'}</b></div>
          <div><span>服务状态</span>{cdk && <StatusPill tone={cdkStatusMeta(cdk.status).tone}>{cdkStatusMeta(cdk.status).label}</StatusPill>}</div>
          <div><span>到期时间</span><b>{formatTime(cdk?.expires_at ?? null)}</b></div>
        </div>
      </section>
      <section className="surface">
        <SectionHeading title="服务与账单" detail="接入信息与额度概览。" />
        <div className="endpoint-row">
          <code>POST {docsURL}/v1/captcha/solve</code>
          <CopyButton compact label="复制地址" text={`${docsURL}/v1/captcha/solve`} />
        </div>
        <div className="definition-list">
          <div><span>平台服务</span><b><StatusPill tone="success">运行正常</StatusPill></b></div>
          <div><span>额度总量</span><b className="mono">{formatCount(cdk?.quota_total ?? 0)}</b></div>
          <div><span>已使用</span><b className="mono">{formatCount(cdk?.quota_used ?? 0)}</b></div>
          <div><span>剩余可用</span><b className="mono">{formatCount(cdk?.quota_remaining ?? 0)}</b></div>
        </div>
        <p className="field-hint">额度充值或续期请联系管理员，通过配额流水完成。</p>
      </section>
    </div>
  );
}

// ---------- docs (static, origin-aware, section switching) ----------

type DocsTab = 'quickstart' | 'authentication' | 'request' | 'response';

const docsTabs: Array<{ id: DocsTab; label: string }> = [
  { id: 'quickstart', label: '快速开始' },
  { id: 'authentication', label: '认证方式' },
  { id: 'request', label: '请求参数' },
  { id: 'response', label: '响应与错误' },
];

function DocsPage() {
  const [snippet, setSnippet] = useState(`curl -X POST ${window.location.origin}/v1/captcha/solve \\
  -H "Authorization: Bearer cf_live_your_key" \\
  -H "Content-Type: application/json" \\
  -H "Idempotency-Key: your-unique-request-id" \\
  -d '{"captcha_id":"captcha_id","risk_type":"slide"}'`);
  const [active, setActive] = useState<DocsTab>('quickstart');

  // Snippets show the operator-configured display base URL when set.
  useEffect(() => {
    void getDocsBaseURL().then((base) => {
      setSnippet(`curl -X POST ${base}/v1/captcha/solve \\
  -H "Authorization: Bearer cf_live_your_key" \\
  -H "Content-Type: application/json" \\
  -H "Idempotency-Key: your-unique-request-id" \\
  -d '{"captcha_id":"captcha_id","risk_type":"slide"}'`);
    });
  }, []);
  // Switch panels instead of scrolling one long page; keep the scroll
  // position sane when the new panel is shorter.
  const select = (id: DocsTab) => {
    setActive(id);
    document.querySelector('.content')?.scrollTo({ top: 0 });
  };
  return (
    <section className="docs-layout">
      <article className="surface docs-toc"><p className="eyebrow">快速导航</p>{docsTabs.map((tab) => <a key={tab.id} aria-current={active === tab.id ? 'page' : undefined} className={active === tab.id ? 'docs-toc__active' : undefined} href={`#${tab.id}`} onClick={(event) => { event.preventDefault(); select(tab.id); }}>{tab.label}</a>)}</article>
      <div className="docs-content">
        {active === 'quickstart' && <article className="surface"><SectionHeading title="快速开始" detail="使用平台发放的 API Key 调用统一的解析接口。" /><div className="doc-callout"><ShieldCheck aria-hidden="true" size={19} /><p>服务端保存底层解析服务凭据。浏览器与用户程序只使用平台 API Key。</p></div></article>}
        {active === 'authentication' && <article className="surface"><SectionHeading title="认证方式" detail="请求头使用 Bearer Token，并为每次请求提供唯一的幂等键。" /><div className="code-panel"><div><span>cURL</span><CopyButton compact label="复制示例" text={snippet} /></div><pre><code>{snippet}</code></pre></div></article>}
        {active === 'request' && <article className="surface"><SectionHeading title="请求参数" /><div className="table-wrap"><table><thead><tr><th>字段</th><th>类型</th><th>必填</th><th>说明</th></tr></thead><tbody><tr><td><code>captcha_id</code></td><td>string</td><td>是</td><td>目标验证码标识</td></tr><tr><td><code>risk_type</code></td><td>string</td><td>是</td><td>V1 固定为 <code>slide</code></td></tr></tbody></table></div></article>}
        {active === 'response' && <article className="surface"><SectionHeading title="响应与错误" detail="所有成功和失败响应都携带平台 request_id。" /><div className="error-grid"><div><StatusPill tone="success">200</StatusPill><p>解析成功，确认消耗一单位额度。</p></div><div><StatusPill tone="warning">429</StatusPill><p>速率或并发超限，携带 Retry-After。</p></div><div><StatusPill tone="warning">402</StatusPill><p>额度耗尽，需联系管理员补充。</p></div><div><StatusPill tone="danger">502</StatusPill><p>下游异常，自动退回预扣额度。</p></div></div></article>}
      </div>
    </section>
  );
}

/** Escape closes a modal; backdrop click is handled per-dialog. */
function useEscapeToClose(onClose: () => void) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);
}

export function App() {
  // The URL is the single source of truth: refresh, back/forward and deep
  // links all resolve through the history router.
  const route = useRoute();
  const [sessionEpoch, setSessionEpoch] = useState(0);
  // Session probe result for console routes: null = still checking. Admin
  // routes manage their own login gate inside AdminPage.
  const [consoleSession, setConsoleSession] = useState<'checking' | 'guest' | 'authenticated'>('checking');

  // Any API 401 (expired/revoked session) forces a fresh probe; the console
  // route then redirects to activation instead of endless load errors.
  useEffect(() => {
    const onUnauthorized = () => setSessionEpoch((value) => value + 1);
    window.addEventListener('captchaflow:unauthorized', onUnauthorized);
    return () => window.removeEventListener('captchaflow:unauthorized', onUnauthorized);
  }, []);

  // Probe the session on every named route so public headers can offer a
  // console shortcut too; navigation inside the console (same route name)
  // does not re-probe.
  useEffect(() => {
    let alive = true;
    setConsoleSession('checking');
    void (async () => {
      try {
        await api.account();
        if (alive) setConsoleSession('authenticated');
      } catch {
        if (alive) setConsoleSession('guest');
      }
    })();
    return () => { alive = false; };
  }, [route.name, sessionEpoch]);

  if (route.name === 'activate') {
    return (
      <AuthPage
        onBack={() => navigate('/')}
        onDocs={() => navigate('/docs')}
        onSuccess={() => { setSessionEpoch((value) => value + 1); navigate(consolePath('dashboard')); }}
      />
    );
  }
  if (route.name === 'docs') {
    return <PublicDocs onBack={() => navigate('/')} onStart={() => navigate('/activate')} session={consoleSession} onConsole={() => navigate(consolePath('dashboard'))} />;
  }
  if (route.name === 'home') {
    return (
      <PublicHome
        onAdmin={() => navigate('/admin')}
        onDocs={() => navigate('/docs')}
        onStart={() => navigate('/activate')}
        session={consoleSession}
        onConsole={() => navigate(consolePath('dashboard'))}
      />
    );
  }
  if (route.name === 'admin') {
    return <AdminPage initialSection={route.section} onExit={() => navigate('/')} />;
  }

  // Console route with the session still being probed.
  if (consoleSession === 'checking') {
    return <div className="public-shell" style={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}><div className="empty-state"><RefreshCw aria-hidden="true" size={28} /><strong>正在进入平台…</strong></div></div>;
  }
  // Guest users on a console URL are sent to activation; replace keeps the
  // guarded URL out of history so back returns to the previous public page.
  if (consoleSession === 'guest') {
    return <AuthPage onBack={() => navigate('/')} onDocs={() => navigate('/docs')} onSuccess={() => { setSessionEpoch((value) => value + 1); navigate(consolePath('dashboard'), true); }} />;
  }

  const activePage: ConsolePage = route.page;
  const activeTitle = pageTitles[activePage];

  const renderPage = () => {
    switch (activePage) {
      case 'dashboard': return <Dashboard goTo={(page) => navigate(consolePath(page))} />;
      case 'keys': return <KeysPage />;
      case 'debug': return <DebugPage />;
      case 'usage': return <UsagePage />;
      case 'calls': return <CallsPage />;
      case 'docs': return <DocsPage />;
      case 'account': return <AccountPage />;
    }
  };

  const logout = async () => {
    try {
      await api.logout();
    } finally {
      setSessionEpoch((value) => value + 1);
      navigate('/');
    }
  };

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <button className="brand" onClick={() => navigate('/')} title="返回首页" type="button"><span className="brand__mark"><ShieldCheck aria-hidden="true" size={21} /></span><span>CaptchaFlow<br /><b>Service Platform</b></span></button>
        <div className="workspace-label">用户控制台</div>
        <nav aria-label="主导航">
          {navigation.map((item) => {
            const Icon = item.icon;
            return <button className={`nav-item ${activePage === item.id ? 'nav-item--active' : ''}`} key={item.id} onClick={() => navigate(consolePath(item.id))} type="button"><Icon aria-hidden="true" size={18} /><span>{item.label}</span></button>;
          })}
        </nav>
        <div className="sidebar__bottom"><div className="service-mini"><span><Activity aria-hidden="true" size={15} />服务运行正常</span><small>API 可用</small></div><button className="sidebar-help" onClick={() => navigate(consolePath('account'))} type="button"><CircleUserRound aria-hidden="true" size={17} />账号与服务</button><button className="sidebar-logout" onClick={() => void logout()} type="button"><LogOut aria-hidden="true" size={17} />退出登录</button></div>
      </aside>
      <div className="app-main">
        <header className="topbar"><div className="breadcrumb"><span>用户控制台</span><ChevronRight aria-hidden="true" size={15} /><b>{activeTitle.title}</b></div><div className="topbar__actions"><StatusPill tone="success">平台服务正常</StatusPill></div></header>
        <main className="content"><div className="page-title"><div><p className="eyebrow">CAPTCHA SERVICE</p><h1>{activeTitle.title}</h1><p>{activeTitle.description}</p></div>{activePage === 'dashboard' && <button className="primary-button page-title__button" onClick={() => navigate(consolePath('keys'))} type="button"><Plus aria-hidden="true" size={17} />新建 API Key</button>}</div>{renderPage()}</main>
      </div>
    </div>
  );
}

export default App;
