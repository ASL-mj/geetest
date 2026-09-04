import {
  ArrowLeft,
  CheckCircle2,
  CreditCard,
  FileText,
  KeyRound,
  LayoutDashboard,
  Plus,
  RefreshCw,
  ServerCog,
  ShieldAlert,
  Users,
} from 'lucide-react';
import { useEffect, useState } from 'react';

import { adminApi, ApiError, type AdminAuditEntry, type AdminBatch, type AdminCdk, type AdminDashboard, type AdminUser } from '../lib/api';
import { cdkStatusMeta, formatCount, formatTime, type StatusTone } from '../lib/status';

type AdminSection = 'overview' | 'batches' | 'cdks' | 'users' | 'audit' | 'health';

const adminNavigation: Array<{ id: AdminSection; label: string; icon: typeof LayoutDashboard }> = [
  { id: 'overview', label: '数据概览', icon: LayoutDashboard },
  { id: 'batches', label: 'CDK 批次', icon: CreditCard },
  { id: 'cdks', label: 'CDK 管理', icon: KeyRound },
  { id: 'users', label: '用户管理', icon: Users },
  { id: 'audit', label: '管理员审计', icon: FileText },
  { id: 'health', label: '服务状态', icon: ServerCog },
];

function AdminStatus({ children, tone = 'success' }: { children: string; tone?: StatusTone }) {
  return <span className={`status-pill status-pill--${tone}`}>{children}</span>;
}

function AdminError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="empty-state" role="alert">
      <RefreshCw aria-hidden="true" size={24} />
      <strong>加载失败</strong>
      <p>{message}</p>
      <button className="quiet-button" onClick={onRetry} type="button">重试</button>
    </div>
  );
}

function OverviewSection() {
  const [dashboard, setDashboard] = useState<AdminDashboard | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = () => {
    setError(null);
    void adminApi.dashboard()
      .then((envelope) => setDashboard(envelope.data ?? null))
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'));
  };
  useEffect(() => { load(); }, []);

  if (error) return <AdminError message={error} onRetry={load} />;
  if (!dashboard) return <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在加载…</strong></div>;

  return (
    <>
      <div className="admin-metric-grid">
        <article className="admin-metric"><span>用户总数</span><strong>{formatCount(dashboard.users_total)}</strong><small>活跃 {formatCount(dashboard.users_active)}</small></article>
        <article className="admin-metric admin-metric--blue"><span>今日调用</span><strong>{formatCount(dashboard.calls_today)}</strong><small>累计消耗 {formatCount(dashboard.quota_consumed)} 次额度</small></article>
        <article className="admin-metric admin-metric--green"><span>成功率</span><strong>{(dashboard.success_rate * 100).toFixed(1)}%</strong><small>今日已结算调用</small></article>
        <article className="admin-metric admin-metric--orange"><span>异常请求</span><strong>{formatCount(dashboard.calls_failed_today)}</strong><small>失败已自动退回额度</small></article>
      </div>
      <section className="admin-panel">
        <div className="admin-panel__heading"><div><h2>CDK 状态分布</h2><p>平台服务容量速览</p></div></div>
        <div className="definition-list">
          <div><span>激活可用</span><b>{formatCount(dashboard.cdks_active)}</b></div>
          <div><span>未激活</span><b>{formatCount(dashboard.cdks_unactivated)}</b></div>
          <div><span>额度耗尽</span><b>{formatCount(dashboard.cdks_exhausted)}</b></div>
        </div>
      </section>
    </>
  );
}

function BatchesSection() {
  const [batches, setBatches] = useState<AdminBatch[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [generated, setGenerated] = useState<string[] | null>(null);

  const load = () => {
    setError(null);
    void adminApi.listBatches()
      .then((envelope) => setBatches(envelope.data?.items ?? []))
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'));
  };
  useEffect(() => { load(); }, []);

  return (
    <section className="admin-panel">
      <div className="admin-panel__heading">
        <div><h2>CDK 批次</h2><p>明文兑换码只在生成响应中出现一次。</p></div>
        <button className="public-primary" onClick={() => setShowCreate(true)} type="button"><Plus aria-hidden="true" size={16} />新建批次</button>
      </div>
      {error && <AdminError message={error} onRetry={load} />}
      {batches === null && !error && <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在加载…</strong></div>}
      {batches !== null && (
        <div className="admin-table-wrap">
          <table>
            <thead><tr><th>批次名称</th><th>默认额度</th><th>CDK 总数</th><th>激活数</th><th>创建时间</th></tr></thead>
            <tbody>
              {batches.map((batch) => (
                <tr key={batch.id}>
                  <td><strong className="admin-mono">{batch.name}</strong></td>
                  <td className="admin-mono">{formatCount(batch.default_quota)}</td>
                  <td className="admin-mono">{formatCount(batch.total_cdks)}</td>
                  <td className="admin-mono">{formatCount(batch.active_cdks)}</td>
                  <td>{formatTime(batch.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {showCreate && (
        <BatchDialog
          onClose={() => setShowCreate(false)}
          onCreated={(codes) => { setGenerated(codes); setShowCreate(false); load(); }}
        />
      )}
      {generated && (
        <div className="dialog-backdrop" role="presentation">
          <section aria-labelledby="codes-title" aria-modal="true" className="dialog" role="dialog">
            <div className="dialog__header"><div><h2 id="codes-title">兑换码已生成</h2><p>关闭后无法再次查看，请立即导出。</p></div></div>
            <div className="secret-field"><code>{generated.join('\n')}</code></div>
            <div className="dialog__actions">
              <button className="quiet-button" onClick={() => void navigator.clipboard?.writeText(generated.join('\n'))} type="button">复制全部</button>
              <button className="primary-button" onClick={() => setGenerated(null)} type="button">我已导出</button>
            </div>
          </section>
        </div>
      )}
    </section>
  );
}

function BatchDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (codes: string[]) => void }) {
  const [name, setName] = useState('');
  const [quota, setQuota] = useState(100);
  const [count, setCount] = useState(10);
  const [duration, setDuration] = useState(365);
  const [reason, setReason] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const create = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const envelope = await adminApi.createBatch({ name: name.trim(), quota, count, service_duration_days: duration, reason: reason.trim() });
      onCreated(envelope.data?.codes ?? []);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '生成失败。');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="dialog-backdrop" role="presentation">
      <section aria-labelledby="batch-title" aria-modal="true" className="dialog" role="dialog">
        <div className="dialog__header"><div><h2 id="batch-title">新建 CDK 批次</h2><p>生成后明文只展示一次。</p></div><button className="icon-button" onClick={onClose} type="button">×</button></div>
        <label htmlFor="batch-name">批次名称</label>
        <input id="batch-name" onChange={(event) => setName(event.target.value)} value={name} />
        <label htmlFor="batch-quota">每枚额度</label>
        <input id="batch-quota" min={1} onChange={(event) => setQuota(Number(event.target.value))} type="number" value={quota} />
        <label htmlFor="batch-count">生成数量（1-500）</label>
        <input id="batch-count" max={500} min={1} onChange={(event) => setCount(Number(event.target.value))} type="number" value={count} />
        <label htmlFor="batch-duration">服务时长（天）</label>
        <input id="batch-duration" min={1} onChange={(event) => setDuration(Number(event.target.value))} type="number" value={duration} />
        <label htmlFor="batch-reason">操作原因</label>
        <input id="batch-reason" onChange={(event) => setReason(event.target.value)} placeholder="例如：渠道商订单 42" value={reason} />
        {error && <p className="field-hint" role="alert">{error}</p>}
        <div className="dialog__actions">
          <button className="quiet-button" onClick={onClose} type="button">取消</button>
          <button className="primary-button" disabled={!name.trim() || !reason.trim() || submitting} onClick={() => void create()} type="button">{submitting ? '生成中…' : '生成批次'}</button>
        </div>
      </section>
    </div>
  );
}

function CdksSection() {
  const [cdks, setCdks] = useState<AdminCdk[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = () => {
    setError(null);
    void adminApi.listCdks()
      .then((envelope) => setCdks(envelope.data?.items ?? []))
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'));
  };
  useEffect(() => { load(); }, []);

  const adjust = async (cdk: AdminCdk) => {
    const input = window.prompt(`为 ${cdk.code_prefix} 调整额度，输入增量（正数补充，负数扣减）：`);
    if (!input) return;
    const reason = window.prompt('操作原因：');
    if (!reason) return;
    try {
      await adminApi.adjustQuota(cdk.id, Number(input), reason);
      load();
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : '调整失败。');
    }
  };

  if (error) return <AdminError message={error} onRetry={load} />;
  return (
    <section className="admin-panel">
      <div className="admin-panel__heading"><div><h2>CDK 管理</h2><p>点击「调整」写入配额流水并生成审计记录。</p></div></div>
      {cdks === null
        ? <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在加载…</strong></div>
        : (
          <div className="admin-table-wrap">
            <table>
              <thead><tr><th>前缀</th><th>状态</th><th>绑定用户</th><th>额度</th><th>剩余</th><th>激活时间</th><th aria-label="操作" /></tr></thead>
              <tbody>
                {cdks.map((cdk) => {
                  const meta = cdkStatusMeta(cdk.status);
                  return (
                    <tr key={cdk.id}>
                      <td><span className="admin-mono">{cdk.code_prefix}</span></td>
                      <td><AdminStatus tone={meta.tone}>{meta.label}</AdminStatus></td>
                      <td><span className="admin-mono">{cdk.bound_user_id ? cdk.bound_user_id.slice(0, 8) : '—'}</span></td>
                      <td className="admin-mono">{formatCount(cdk.quota_total)}</td>
                      <td className="admin-mono">{formatCount(cdk.quota_remaining)}</td>
                      <td>{formatTime(cdk.activated_at)}</td>
                      <td><button className="icon-button" onClick={() => void adjust(cdk)} title="调整额度" type="button"><Plus aria-hidden="true" size={16} /></button></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
    </section>
  );
}

function UsersSection() {
  const [users, setUsers] = useState<AdminUser[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = () => {
    setError(null);
    void adminApi.listUsers()
      .then((envelope) => setUsers(envelope.data?.items ?? []))
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'));
  };
  useEffect(() => { load(); }, []);

  const setStatus = async (user: AdminUser) => {
    const next = user.status === 'ACTIVE' ? 'SUSPENDED' : 'ACTIVE';
    const reason = window.prompt(next === 'SUSPENDED' ? '封禁原因：' : '恢复原因：');
    if (!reason) return;
    try {
      await adminApi.setUserStatus(user.id, next, reason);
      load();
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : '操作失败。');
    }
  };

  if (error) return <AdminError message={error} onRetry={load} />;
  return (
    <section className="admin-panel">
      <div className="admin-panel__heading"><div><h2>用户管理</h2><p>封禁立即撤销该用户全部会话。</p></div></div>
      {users === null
        ? <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在加载…</strong></div>
        : (
          <div className="admin-table-wrap">
            <table>
              <thead><tr><th>用户 ID</th><th>CDK 前缀</th><th>状态</th><th>剩余额度</th><th>创建时间</th><th aria-label="操作" /></tr></thead>
              <tbody>
                {users.map((user) => (
                  <tr key={user.id}>
                    <td><span className="admin-mono">{user.id.slice(0, 8)}</span></td>
                    <td><span className="admin-mono">{user.cdk_prefix ?? '—'}</span></td>
                    <td><AdminStatus tone={user.status === 'ACTIVE' ? 'success' : 'danger'}>{user.status === 'ACTIVE' ? '正常' : '已封禁'}</AdminStatus></td>
                    <td className="admin-mono">{user.cdk_remaining != null ? formatCount(user.cdk_remaining) : '—'}</td>
                    <td>{formatTime(user.created_at)}</td>
                    <td>
                      <button className="icon-button" onClick={() => void setStatus(user)} title={user.status === 'ACTIVE' ? '封禁' : '恢复'} type="button">
                        {user.status === 'ACTIVE' ? '封禁' : '恢复'}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
    </section>
  );
}

function AuditSection() {
  const [entries, setEntries] = useState<AdminAuditEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = () => {
    setError(null);
    void adminApi.listAuditLogs()
      .then((envelope) => setEntries(envelope.data?.items ?? []))
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'));
  };
  useEffect(() => { load(); }, []);

  if (error) return <AdminError message={error} onRetry={load} />;
  return (
    <section className="admin-panel">
      <div className="admin-panel__heading"><div><h2>管理员审计</h2><p>所有写操作都会生成不可篡改的审计记录。</p></div></div>
      {entries === null
        ? <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在加载…</strong></div>
        : entries.length === 0
          ? <div className="empty-state"><CheckCircle2 aria-hidden="true" size={26} /><strong>暂无审计记录</strong></div>
          : (
            <div className="admin-table-wrap">
              <table>
                <thead><tr><th>动作</th><th>目标</th><th>原因</th><th>脱敏 IP</th><th>时间</th></tr></thead>
                <tbody>
                  {entries.map((entry) => (
                    <tr key={entry.id}>
                      <td><strong className="admin-mono">{entry.action}</strong></td>
                      <td><span className="admin-mono">{entry.target_type}:{String(entry.target_id).slice(0, 8)}</span></td>
                      <td>{entry.reason}</td>
                      <td><span className="admin-mono">{entry.ip_masked || '—'}</span></td>
                      <td>{formatTime(entry.created_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
    </section>
  );
}

function HealthSection() {
  const [health, setHealth] = useState<{ status: string; latency_ms: number; checked_at: string; failure?: string } | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = () => {
    setError(null);
    void adminApi.solverHealth()
      .then((envelope) => setHealth(envelope.data ?? null))
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'));
  };
  useEffect(() => { load(); }, []);

  if (error) return <AdminError message={error} onRetry={load} />;
  if (!health) return <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在检测…</strong></div>;
  const tone = health.status === 'OK' ? 'success' : health.status === 'DEGRADED' ? 'warning' : 'danger';
  return (
    <section className="admin-panel">
      <div className="admin-panel__heading"><div><h2>底层解析服务状态</h2><p>超时 3 秒；只报告状态与延迟，不暴露服务地址。</p></div></div>
      <div className="definition-list">
        <div><span>状态</span><AdminStatus tone={tone}>{health.status}</AdminStatus></div>
        <div><span>延迟</span><b>{health.latency_ms} ms</b></div>
        {health.failure && <div><span>失败分类</span><b>{health.failure}</b></div>}
        <div><span>检测时间</span><b>{formatTime(health.checked_at)}</b></div>
      </div>
    </section>
  );
}

function AdminLoginGate({ onAuthenticated }: { onAuthenticated: () => void }) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await adminApi.login(username.trim(), password);
      onAuthenticated();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '登录失败，请稍后重试。');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="public-shell" style={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}>
      <form className="dialog" onSubmit={submit} style={{ minWidth: 320 }}>
        <div className="dialog__header"><div><h2>管理员登录</h2><p>使用运营账号进入平台后台。</p></div></div>
        <label htmlFor="admin-username">用户名</label>
        <input autoFocus id="admin-username" onChange={(event) => setUsername(event.target.value)} value={username} />
        <label htmlFor="admin-password">密码</label>
        <input autoComplete="current-password" id="admin-password" onChange={(event) => setPassword(event.target.value)} type="password" value={password} />
        {error && <p className="field-hint" role="alert">{error}</p>}
        <div className="dialog__actions">
          <button className="primary-button" disabled={!username.trim() || !password || submitting} type="submit">
            {submitting ? '登录中…' : '登录'}
          </button>
        </div>
      </form>
    </div>
  );
}

export function AdminPage({ onExit }: { onExit: () => void }) {
  const [section, setSection] = useState<AdminSection>('overview');
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const current = adminNavigation.find((item) => item.id === section) ?? adminNavigation[0];

  // Probe whether an operator cookie already exists.
  useEffect(() => {
    let alive = true;
    void adminApi.dashboard()
      .then(() => { if (alive) setAuthenticated(true); })
      .catch(() => { if (alive) setAuthenticated(false); });
    return () => { alive = false; };
  }, []);

  if (authenticated === null) {
    return <div className="public-shell" style={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}><div className="empty-state"><RefreshCw aria-hidden="true" size={26} /><strong>正在验证…</strong></div></div>;
  }
  if (!authenticated) {
    return <AdminLoginGate onAuthenticated={() => setAuthenticated(true)} />;
  }

  return (
    <div className="admin-shell">
      <aside className="admin-sidebar">
        <div className="admin-brand"><span className="public-brand__mark"><ShieldAlert aria-hidden="true" size={19} /></span><span>平台管理员<br /><b>Operations Console</b></span></div>
        <p className="admin-label">运营管理</p>
        <nav aria-label="管理员导航">
          {adminNavigation.map((item) => {
            const Icon = item.icon;
            return (
              <button className={section === item.id ? 'admin-nav-item admin-nav-item--active' : 'admin-nav-item'} key={item.id} onClick={() => setSection(item.id)} type="button">
                <Icon aria-hidden="true" size={16} />{item.label}
              </button>
            );
          })}
        </nav>
        <button className="admin-exit" onClick={onExit} type="button"><ArrowLeft aria-hidden="true" size={16} />返回用户入口</button>
      </aside>
      <main className="admin-main">
        <header className="admin-topbar"><div><span>管理员后台</span><b> / {current.label}</b></div><div><AdminStatus>已登录</AdminStatus><span className="admin-avatar">AD</span></div></header>
        <div className="admin-content">
          <div className="admin-title">
            <div><p className="public-eyebrow">OPERATIONS CONSOLE</p><h1>{current.label}</h1><p>统一管理 CDK、用户、Key、调用和平台运行状态。</p></div>
          </div>
          {section === 'overview' && <OverviewSection />}
          {section === 'batches' && <BatchesSection />}
          {section === 'cdks' && <CdksSection />}
          {section === 'users' && <UsersSection />}
          {section === 'audit' && <AuditSection />}
          {section === 'health' && <HealthSection />}
        </div>
      </main>
    </div>
  );
}
