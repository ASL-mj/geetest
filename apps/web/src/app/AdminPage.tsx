import {
  ArrowLeft,
  CheckCircle2,
  Copy,
  FileText,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Plus,
  RefreshCw,
  ServerCog,
  Settings,
  ShieldAlert,
  Users,
  Layers3,
} from 'lucide-react';
import { type ReactNode, useEffect, useState } from 'react';

import { adminApi, ApiError, type AdminAuditEntry, type AdminBatch, type AdminCdk, type AdminDashboard, type AdminUser, type SystemConfig } from '../lib/api';
import { adminPath, navigate, useRoute, type AdminSection } from '../lib/router';
import { cdkStatusMeta, formatCount, formatTime, type StatusTone } from '../lib/status';

const adminNavigation: Array<{ id: AdminSection; label: string; icon: typeof LayoutDashboard }> = [
  { id: 'overview', label: '数据概览', icon: LayoutDashboard },
  { id: 'batches', label: 'CDK 批次', icon: Layers3 },
  { id: 'cdks', label: 'CDK 管理', icon: KeyRound },
  { id: 'users', label: '用户管理', icon: Users },
  { id: 'audit', label: '管理员审计', icon: FileText },
  { id: 'health', label: '服务状态', icon: ServerCog },
  { id: 'system', label: '系统配置', icon: Settings },
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

// AdminDialog replaces window.prompt/alert flows with proper modals.
function AdminDialog({ title, description, onClose, children }: { title: string; description?: string; onClose: () => void; children: ReactNode }) {
  return (
    <div className="dialog-backdrop" role="presentation">
      <section aria-label={title} aria-modal="true" className="dialog" role="dialog">
        <div className="dialog__header"><div><h2>{title}</h2>{description && <p>{description}</p>}</div><button className="icon-button" onClick={onClose} title="关闭" type="button">×</button></div>
        {children}
      </section>
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

// ---------- CDK management (creation + lifecycle in one view) ----------

function CdksSection() {
  const [cdks, setCdks] = useState<AdminCdk[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [showSingle, setShowSingle] = useState(false);
  const [showBatch, setShowBatch] = useState(false);
  const [generated, setGenerated] = useState<string[] | null>(null);
  const [adjustTarget, setAdjustTarget] = useState<AdminCdk | null>(null);
  const [statusTarget, setStatusTarget] = useState<AdminCdk | null>(null);
  const [banTarget, setBanTarget] = useState<AdminCdk | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = () => {
    setError(null);
    void adminApi.listCdks()
      .then((envelope) => setCdks(envelope.data?.items ?? []))
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'));
  };
  useEffect(() => { load(); }, []);

  const revealCode = async (cdk: AdminCdk) => {
    try {
      const envelope = await adminApi.revealCdkCode(cdk.id);
      const code = envelope.data?.code ?? '';
      if (!code) throw new Error('empty');
      setGenerated([code]);
      navigator.clipboard?.writeText(code).catch(() => {});
    } catch (err) {
      setNotice(err instanceof ApiError ? err.message : '无法获取兑换码明文。');
    }
  };

  if (error) return <AdminError message={error} onRetry={load} />;
  return (
    <section className="admin-panel">
      <div className="admin-panel__heading">
        <div><h2>CDK 管理</h2><p>创建、调整、禁用一条完成；明文兑换码可随时再次复制。</p></div>
        <div className="admin-panel__actions">
          <button className="quiet-button" onClick={() => setShowSingle(true)} type="button"><Plus aria-hidden="true" size={15} />单条创建</button>
          <button className="public-primary" onClick={() => setShowBatch(true)} type="button"><Plus aria-hidden="true" size={16} />批量创建</button>
        </div>
      </div>
      {notice && <div className="admin-inline-error" role="alert"><ShieldAlert aria-hidden="true" size={15} />{notice}<button onClick={() => setNotice(null)} type="button">知道了</button></div>}
      {cdks === null
        ? <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在加载…</strong></div>
        : (
          <div className="admin-table-wrap">
            <table>
              <thead><tr><th>前缀</th><th>状态</th><th>备注</th><th>绑定用户</th><th>额度</th><th>剩余</th><th>激活时间</th><th aria-label="操作" /></tr></thead>
              <tbody>
                {cdks.map((cdk) => {
                  const meta = cdkStatusMeta(cdk.status);
                  const banned = cdk.bound_user_state === 'SUSPENDED';
                  return (
                    <tr key={cdk.id}>
                      <td><span className="admin-mono">{cdk.code_prefix}</span></td>
                      <td><AdminStatus tone={meta.tone}>{meta.label}</AdminStatus></td>
                      <td><span className="admin-remark" title={cdk.remark ?? ''}>{cdk.remark || '—'}</span></td>
                      <td>
                        {cdk.bound_user_id
                          ? <span className="admin-mono">{cdk.bound_user_id.slice(0, 8)} <AdminStatus tone={banned ? 'danger' : 'success'}>{banned ? '已封禁' : '正常'}</AdminStatus></span>
                          : '—'}
                      </td>
                      <td className="admin-mono">{formatCount(cdk.quota_total)}</td>
                      <td className="admin-mono">{formatCount(cdk.quota_remaining)}</td>
                      <td>{formatTime(cdk.activated_at)}</td>
                      <td className="admin-actions">
                        <button className="icon-button" onClick={() => void revealCode(cdk)} title="复制兑换码" type="button"><Copy aria-hidden="true" size={15} /></button>
                        <button className="admin-action" onClick={() => setAdjustTarget(cdk)} type="button">调整</button>
                        <button className="admin-action" onClick={() => setStatusTarget(cdk)} type="button">{cdk.status === 'DISABLED' ? '启用' : '禁用'}</button>
                        {cdk.bound_user_id && (
                          <button className="admin-action admin-action--danger" onClick={() => setBanTarget(cdk)} type="button">{banned ? '恢复' : '封禁'}</button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      {showSingle && (
        <SingleCdkDialog
          onClose={() => setShowSingle(false)}
          onCreated={(codes) => { setGenerated(codes); setShowSingle(false); load(); }}
        />
      )}
      {showBatch && (
        <BatchDialog
          onClose={() => setShowBatch(false)}
          onCreated={(codes) => { setGenerated(codes); setShowBatch(false); load(); }}
        />
      )}
      {adjustTarget && (
        <AdjustQuotaDialog cdk={adjustTarget} onClose={() => setAdjustTarget(null)} onDone={() => { setAdjustTarget(null); load(); }} />
      )}
      {statusTarget && (
        <ReasonDialog
          title={statusTarget.status === 'DISABLED' ? '启用 CDK' : '禁用 CDK'}
          description={`${statusTarget.code_prefix}… 启用后可重新激活与调用。`}
          confirmLabel={statusTarget.status === 'DISABLED' ? '启用' : '禁用'}
          onClose={() => setStatusTarget(null)}
          onSubmit={async (reason) => {
            await adminApi.setCdkStatus(statusTarget.id, statusTarget.status === 'DISABLED' ? 'ACTIVE' : 'DISABLED', reason);
          }}
          onDone={() => { setStatusTarget(null); load(); }}
        />
      )}
      {banTarget && banTarget.bound_user_id && (
        <ReasonDialog
          title={banTarget.bound_user_state === 'SUSPENDED' ? '恢复用户' : '封禁用户'}
          description={`绑定用户 ${banTarget.bound_user_id.slice(0, 8)}… 封禁会立即撤销其全部会话。`}
          confirmLabel={banTarget.bound_user_state === 'SUSPENDED' ? '恢复' : '封禁'}
          onClose={() => setBanTarget(null)}
          onSubmit={async (reason) => {
            await adminApi.setUserStatus(banTarget.bound_user_id!, banTarget.bound_user_state === 'SUSPENDED' ? 'ACTIVE' : 'SUSPENDED', reason);
          }}
          onDone={() => { setBanTarget(null); load(); }}
        />
      )}
      {generated && (
        <AdminDialog description="已复制到剪贴板；也可以在列表中随时再次复制。" onClose={() => setGenerated(null)} title="兑换码明文">
          <div className="secret-field"><code>{generated.join('\n')}</code></div>
          <div className="dialog__actions">
            <button className="quiet-button" onClick={() => navigator.clipboard?.writeText(generated.join('\n')).catch(() => {})} type="button">复制全部</button>
            <button className="primary-button" onClick={() => setGenerated(null)} type="button">完成</button>
          </div>
        </AdminDialog>
      )}
    </section>
  );
}

function BatchesSection() {
  const [batches, setBatches] = useState<AdminBatch[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const load = () => {
    setError(null);
    void adminApi.listBatches()
      .then((envelope) => setBatches(envelope.data?.items ?? []))
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'));
  };
  useEffect(() => { load(); }, []);
  if (error) return <AdminError message={error} onRetry={load} />;
  return <section className="admin-panel">
    <div className="admin-panel__heading"><div><h2>CDK 批次</h2><p>按批次查看发放规模、默认额度和激活情况。</p></div></div>
    {batches === null ? <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在加载…</strong></div>
      : batches.length === 0 ? <div className="empty-state"><Layers3 aria-hidden="true" size={26} /><strong>暂无批次</strong></div>
      : <div className="admin-table-wrap"><table><thead><tr><th>批次名称</th><th>默认额度</th><th>CDK 数量</th><th>已激活</th><th>创建时间</th></tr></thead><tbody>
        {batches.map((batch) => <tr key={batch.id}><td><strong>{batch.name}</strong></td><td className="admin-mono">{formatCount(batch.default_quota)}</td><td className="admin-mono">{formatCount(batch.total_cdks)}</td><td className="admin-mono">{formatCount(batch.active_cdks)}</td><td>{formatTime(batch.created_at)}</td></tr>)}
      </tbody></table></div>}
  </section>;
}

function UsersSection() {
  const [users, setUsers] = useState<AdminUser[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [target, setTarget] = useState<AdminUser | null>(null);
  const load = () => {
    setError(null);
    void adminApi.listUsers()
      .then((envelope) => setUsers(envelope.data?.items ?? []))
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'));
  };
  useEffect(() => { load(); }, []);
  if (error) return <AdminError message={error} onRetry={load} />;
  return <section className="admin-panel">
    <div className="admin-panel__heading"><div><h2>用户管理</h2><p>查看 CDK 绑定与剩余额度，封禁会立即撤销用户会话。</p></div></div>
    {users === null ? <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在加载…</strong></div>
      : users.length === 0 ? <div className="empty-state"><Users aria-hidden="true" size={26} /><strong>暂无用户</strong></div>
      : <div className="admin-table-wrap"><table><thead><tr><th>用户 ID</th><th>状态</th><th>绑定 CDK</th><th>CDK 状态</th><th>剩余额度</th><th aria-label="操作" /></tr></thead><tbody>
        {users.map((user) => <tr key={user.id}><td><span className="admin-mono">{user.id.slice(0, 12)}…</span></td><td><AdminStatus tone={user.status === 'ACTIVE' ? 'success' : 'danger'}>{user.status === 'ACTIVE' ? '正常' : '已封禁'}</AdminStatus></td><td className="admin-mono">{user.cdk_prefix ?? '—'}</td><td>{user.cdk_status ?? '—'}</td><td className="admin-mono">{user.cdk_remaining == null ? '—' : formatCount(user.cdk_remaining)}</td><td className="admin-actions"><button className="admin-action admin-action--danger" onClick={() => setTarget(user)} type="button">{user.status === 'ACTIVE' ? '封禁' : '恢复'}</button></td></tr>)}
      </tbody></table></div>}
    {target && <ReasonDialog title={target.status === 'ACTIVE' ? '封禁用户' : '恢复用户'} description="操作会写入管理员审计日志。" confirmLabel={target.status === 'ACTIVE' ? '封禁' : '恢复'} onClose={() => setTarget(null)} onSubmit={(reason) => adminApi.setUserStatus(target.id, target.status === 'ACTIVE' ? 'SUSPENDED' : 'ACTIVE', reason).then(() => undefined)} onDone={() => { setTarget(null); load(); }} />}
  </section>;
}

function SingleCdkDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (codes: string[]) => void }) {
  const [remark, setRemark] = useState('');
  const [quota, setQuota] = useState(100);
  const [duration, setDuration] = useState(365);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const create = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const envelope = await adminApi.createBatch({ name: remark.trim() || '单条创建', quota, count: 1, service_duration_days: duration, reason: remark.trim() || '单条创建' });
      onCreated(envelope.data?.codes ?? []);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '生成失败。');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AdminDialog description="生成后可在列表中随时复制明文。" onClose={onClose} title="单条创建 CDK">
      <label htmlFor="single-remark">备注</label>
      <input id="single-remark" onChange={(event) => setRemark(event.target.value)} placeholder="例如：渠道 A 客户张三" value={remark} />
      <label htmlFor="single-quota">额度次数</label>
      <input id="single-quota" min={1} onChange={(event) => setQuota(Number(event.target.value))} type="number" value={quota} />
      <label htmlFor="single-duration">服务时长（天）</label>
      <input id="single-duration" min={1} onChange={(event) => setDuration(Number(event.target.value))} type="number" value={duration} />
      {error && <p className="field-hint" role="alert">{error}</p>}
      <div className="dialog__actions">
        <button className="quiet-button" onClick={onClose} type="button">取消</button>
        <button className="primary-button" disabled={submitting} onClick={() => void create()} type="button">{submitting ? '生成中…' : '生成 CDK'}</button>
      </div>
    </AdminDialog>
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
    <AdminDialog description="生成后明文在列表中可随时复制。" onClose={onClose} title="批量创建 CDK">
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
    </AdminDialog>
  );
}

function AdjustQuotaDialog({ cdk, onClose, onDone }: { cdk: AdminCdk; onClose: () => void; onDone: () => void }) {
  const [delta, setDelta] = useState('');
  const [reason, setReason] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await adminApi.adjustQuota(cdk.id, Number(delta), reason.trim());
      onDone();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '调整失败。');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AdminDialog description={`${cdk.code_prefix}… 当前剩余 ${formatCount(cdk.quota_remaining)} 次`} onClose={onClose} title="调整额度">
      <label htmlFor="adjust-delta">变动额度（正数补充，负数扣减）</label>
      <input id="adjust-delta" onChange={(event) => setDelta(event.target.value)} placeholder="例如：100 或 -50" type="number" value={delta} />
      <label htmlFor="adjust-reason">操作原因</label>
      <input id="adjust-reason" onChange={(event) => setReason(event.target.value)} placeholder="例如：支持工单 42" value={reason} />
      {error && <p className="field-hint" role="alert">{error}</p>}
      <div className="dialog__actions">
        <button className="quiet-button" onClick={onClose} type="button">取消</button>
        <button className="primary-button" disabled={!delta || Number(delta) === 0 || !reason.trim() || submitting} onClick={() => void submit()} type="button">{submitting ? '提交中…' : '提交调整'}</button>
      </div>
    </AdminDialog>
  );
}

function ReasonDialog({ title, description, confirmLabel, onClose, onSubmit, onDone }: {
  title: string;
  description: string;
  confirmLabel: string;
  onClose: () => void;
  onSubmit: (reason: string) => Promise<void>;
  onDone: () => void;
}) {
  const [reason, setReason] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await onSubmit(reason.trim());
      onDone();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '操作失败。');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AdminDialog description={description} onClose={onClose} title={title}>
      <label htmlFor="reason-input">操作原因</label>
      <input autoFocus id="reason-input" onChange={(event) => setReason(event.target.value)} placeholder="将写入审计日志" value={reason} />
      {error && <p className="field-hint" role="alert">{error}</p>}
      <div className="dialog__actions">
        <button className="quiet-button" onClick={onClose} type="button">取消</button>
        <button className="primary-button" disabled={!reason.trim() || submitting} onClick={() => void submit()} type="button">{submitting ? '提交中…' : confirmLabel}</button>
      </div>
    </AdminDialog>
  );
}

// ---------- audit / health / system ----------

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

function SystemSection() {
  const [config, setConfig] = useState<SystemConfig | null>(null);
  const [baseURL, setBaseURL] = useState('');
  const [reason, setReason] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const load = () => {
    void adminApi.getSystemConfig()
      .then((envelope) => {
        const data = envelope.data ?? { api_base_url: '', has_override: false };
        setConfig(data);
        setBaseURL(data.api_base_url);
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'));
  };
  useEffect(() => { load(); }, []);

  const save = async () => {
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      await adminApi.setSystemConfig({ api_base_url: baseURL.trim(), reason: reason.trim() });
      setSaved(true);
      setReason('');
      load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '保存失败。');
    } finally {
      setSaving(false);
    }
  };

  if (error && !config) return <AdminError message={error} onRetry={load} />;
  return (
    <section className="admin-panel">
      <div className="admin-panel__heading"><div><h2>系统配置</h2><p>展示类配置，保存后立即写入审计。</p></div></div>
      <div className="system-config-form">
        <label htmlFor="api-base-url">平台接入地址（文档展示用）</label>
        <input id="api-base-url" onChange={(event) => setBaseURL(event.target.value)} placeholder="例如：https://api.captchaflow.cn" value={baseURL} />
        <p className="field-hint">
          仅用于公开文档与控制台文档中的示例地址展示；{config?.has_override
            ? <AdminStatus tone="warning">已自定义</AdminStatus>
            : <AdminStatus tone="success">未设置（文档使用当前域名）</AdminStatus>}
          。底层解析服务不受此项影响。
        </p>
        <label htmlFor="api-base-reason">操作原因</label>
        <input id="api-base-reason" onChange={(event) => setReason(event.target.value)} placeholder="将写入审计日志" value={reason} />
        {error && <p className="field-hint" role="alert">{error}</p>}
        {saved && <p className="field-hint system-saved"><CheckCircle2 aria-hidden="true" size={14} />已保存，文档立即使用新地址。</p>}
        <div className="dialog__actions">
          <button className="primary-button" disabled={!baseURL.trim() || !reason.trim() || saving || baseURL === config?.api_base_url} onClick={() => void save()} type="button">{saving ? '保存中…' : '保存配置'}</button>
        </div>
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

export function AdminPage({ initialSection, onExit }: { initialSection?: AdminSection; onExit: () => void }) {
  // Section lives in the URL (/admin/overview, /admin/cdks, …) so refresh
  // and deep links land on the same management view.
  const route = useRoute();
  const section: AdminSection = route.name === 'admin' ? route.section : (initialSection ?? 'overview');
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [loggingOut, setLoggingOut] = useState(false);
  const current = adminNavigation.find((item) => item.id === section) ?? adminNavigation[0];

  const handleLogout = async () => {
    setLoggingOut(true);
    try {
      await adminApi.logout();
    } finally {
      setLoggingOut(false);
      setAuthenticated(false);
    }
  };

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
              <button className={section === item.id ? 'admin-nav-item admin-nav-item--active' : 'admin-nav-item'} key={item.id} onClick={() => navigate(adminPath(item.id))} type="button">
                <Icon aria-hidden="true" size={16} />{item.label}
              </button>
            );
          })}
        </nav>
        <div className="admin-sidebar__bottom">
          <button className="admin-exit" onClick={onExit} type="button"><ArrowLeft aria-hidden="true" size={16} />返回用户入口</button>
          <button className="admin-logout" disabled={loggingOut} onClick={() => void handleLogout()} type="button"><LogOut aria-hidden="true" size={15} />{loggingOut ? '正在退出…' : '退出登录'}</button>
        </div>
      </aside>
      <main className="admin-main">
        <header className="admin-topbar"><div><span>管理员后台</span><b> / {current.label}</b></div><div><AdminStatus>已登录</AdminStatus><span className="admin-avatar">AD</span></div></header>
        <div className="admin-content">
          <div className="admin-title">
            <div><p className="public-eyebrow">OPERATIONS CONSOLE</p><h1>{current.label}</h1><p>统一管理 CDK、用户、调用审计与平台运行配置。</p></div>
          </div>
          {section === 'overview' && <OverviewSection />}
          {section === 'batches' && <BatchesSection />}
          {section === 'cdks' && <CdksSection />}
          {section === 'users' && <UsersSection />}
          {section === 'audit' && <AuditSection />}
          {section === 'health' && <HealthSection />}
          {section === 'system' && <SystemSection />}
        </div>
      </main>
    </div>
  );
}
