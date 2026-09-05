import {
  ArrowLeft,
  CheckCircle2,
  Copy,
  Database,
  FileText,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Plus,
  RefreshCw,
  ServerCog,
  Settings,
  ShieldAlert,
  SlidersHorizontal,
} from 'lucide-react';
import { type ReactNode, useEffect, useState } from 'react';

import { adminApi, ApiError, type AdminAPIKey, type AdminAuditEntry, type AdminCall, type AdminCdk, type AdminDashboard, type AdminQuotaLedgerEntry, type SystemConfig } from '../lib/api';
import { adminPath, navigate, type AdminSection } from '../lib/router';
import { cdkStatusMeta, formatCount, formatTime, type StatusTone } from '../lib/status';

const adminNavigation: Array<{ id: AdminSection; label: string; icon: typeof LayoutDashboard }> = [
  { id: 'overview', label: '数据概览', icon: LayoutDashboard },
  { id: 'cdks', label: 'CDK 管理', icon: KeyRound },
  { id: 'api-keys', label: 'API Key', icon: KeyRound },
  { id: 'calls', label: '调用日志', icon: FileText },
  { id: 'quota-ledger', label: '配额流水', icon: Database },
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
              <thead><tr><th>前缀</th><th>批次</th><th>状态</th><th>备注</th><th>绑定用户</th><th>额度</th><th>剩余</th><th>激活时间</th><th aria-label="操作" /></tr></thead>
              <tbody>
                {cdks.map((cdk) => {
                  const meta = cdkStatusMeta(cdk.status);
                  const banned = cdk.bound_user_state === 'SUSPENDED';
                  return (
                    <tr key={cdk.id}>
                      <td><span className="admin-mono">{cdk.code_prefix}</span></td>
                      <td>{cdk.batch_name}</td>
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

function AdminFilters({ children }: { children: ReactNode }) {
  return <div className="admin-query-filters"><SlidersHorizontal aria-hidden="true" size={15} />{children}</div>;
}

function PageControls({ nextCursor, loading, onNext }: { nextCursor: string | null; loading: boolean; onNext: () => void }) {
  return <div className="admin-pagination"><span>{nextCursor ? '还有更多记录' : '已显示全部匹配记录'}</span><button disabled={!nextCursor || loading} onClick={onNext} title="加载下一页" type="button">{loading ? '…' : '下一页'}</button></div>;
}

function asUTCQueryTime(value: string): string | undefined {
  return value ? new Date(value).toISOString() : undefined;
}

function ApiKeysSection() {
  const [items, setItems] = useState<AdminAPIKey[] | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [userID, setUserID] = useState('');
  const [status, setStatus] = useState('');
  const [prefix, setPrefix] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);

  const load = (cursor?: string, append = false) => {
    setError(null);
    void adminApi.listAPIKeys({ user_id: userID, status, prefix, cursor })
      .then((envelope) => {
        const data = envelope.data ?? { items: [], next_cursor: null };
        setItems((current) => append ? [...(current ?? []), ...data.items] : data.items);
        setNextCursor(data.next_cursor);
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'))
      .finally(() => setLoadingMore(false));
  };
  // Filters are applied explicitly to avoid a request per keystroke.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { load(); }, []);
  const apply = () => load();
  if (error && items === null) return <AdminError message={error} onRetry={load} />;
  return <section className="admin-panel">
    <div className="admin-panel__heading"><div><h2>API Key</h2><p>按状态和前缀定位归属 Key；密钥明文与哈希均不展示。</p></div></div>
    <AdminFilters>
      <select aria-label="Key 状态" className="admin-filter" onChange={(event) => setStatus(event.target.value)} value={status}><option value="">全部状态</option><option value="ACTIVE">启用</option><option value="DISABLED">禁用</option><option value="DELETED">已删除</option></select>
      <input aria-label="归属用户 ID" className="admin-filter" onChange={(event) => setUserID(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') apply(); }} placeholder="归属用户 UUID" value={userID} />
      <input aria-label="Key 前缀" className="admin-filter" onChange={(event) => setPrefix(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') apply(); }} placeholder="Key 前缀" value={prefix} />
      <button className="quiet-button" onClick={apply} type="button">筛选</button>
    </AdminFilters>
    {error && <div className="admin-inline-error" role="alert"><ShieldAlert aria-hidden="true" size={15} />{error}</div>}
    {items === null ? <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在加载…</strong></div>
      : items.length === 0 ? <div className="empty-state"><KeyRound aria-hidden="true" size={26} /><strong>暂无匹配 Key</strong></div>
        : <><div className="admin-table-wrap"><table><thead><tr><th>名称</th><th>Key 标识</th><th>状态</th><th>归属用户</th><th>关联 CDK</th><th>调用次数</th><th>最后使用</th></tr></thead><tbody>
          {items.map((item) => <tr key={item.id}><td><strong>{item.name}</strong></td><td><span className="admin-mono">{item.prefix}…{item.last4}</span></td><td><AdminStatus tone={item.status === 'ACTIVE' ? 'success' : item.status === 'DISABLED' ? 'danger' : 'neutral'}>{item.status}</AdminStatus></td><td><span className="admin-mono">{item.user_id.slice(0, 12)}…</span></td><td><span className="admin-mono">{item.cdk_prefix ?? '—'}</span></td><td className="admin-mono">{formatCount(item.total_calls)}</td><td>{formatTime(item.last_used_at)}</td></tr>)}
        </tbody></table></div><PageControls loading={loadingMore} nextCursor={nextCursor} onNext={() => { if (nextCursor) { setLoadingMore(true); load(nextCursor, true); } }} /></>}
  </section>;
}

function CallsSection() {
  const [items, setItems] = useState<AdminCall[] | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [userID, setUserID] = useState('');
  const [cdkID, setCdkID] = useState('');
  const [keyID, setKeyID] = useState('');
  const [status, setStatus] = useState('');
  const [captchaID, setCaptchaID] = useState('');
  const [requestID, setRequestID] = useState('');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const load = (cursor?: string, append = false) => {
    setError(null);
    void adminApi.listCalls({ user_id: userID, cdk_id: cdkID, api_key_id: keyID, status, captcha_id: captchaID, request_id: requestID, from: asUTCQueryTime(from), to: asUTCQueryTime(to), cursor })
      .then((envelope) => {
        const data = envelope.data ?? { items: [], next_cursor: null };
        setItems((current) => append ? [...(current ?? []), ...data.items] : data.items);
        setNextCursor(data.next_cursor);
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'))
      .finally(() => setLoadingMore(false));
  };
  // Filters are applied explicitly to avoid a request per keystroke.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { load(); }, []);
  if (error && items === null) return <AdminError message={error} onRetry={load} />;
  return <section className="admin-panel">
    <div className="admin-panel__heading"><div><h2>调用日志</h2><p>查询请求状态、耗时、错误摘要和关联归属；不展示敏感解析结果。</p></div></div>
    <AdminFilters>
      <select aria-label="调用状态" className="admin-filter" onChange={(event) => setStatus(event.target.value)} value={status}><option value="">全部状态</option><option value="SUCCEEDED">成功</option><option value="FAILED_REFUNDED">失败已退款</option><option value="REJECTED">已拒绝</option><option value="RESERVED">处理中</option></select>
      <input aria-label="用户 ID" className="admin-filter" onChange={(event) => setUserID(event.target.value)} placeholder="用户 UUID" value={userID} />
      <input aria-label="CDK ID" className="admin-filter" onChange={(event) => setCdkID(event.target.value)} placeholder="CDK UUID" value={cdkID} />
      <input aria-label="API Key ID" className="admin-filter" onChange={(event) => setKeyID(event.target.value)} placeholder="Key UUID" value={keyID} />
      <input aria-label="Captcha ID" className="admin-filter" onChange={(event) => setCaptchaID(event.target.value)} placeholder="Captcha ID" value={captchaID} />
      <input aria-label="请求 ID" className="admin-filter" onChange={(event) => setRequestID(event.target.value)} placeholder="请求 ID" value={requestID} />
      <input aria-label="开始时间" className="admin-filter" onChange={(event) => setFrom(event.target.value)} type="datetime-local" value={from} />
      <input aria-label="结束时间" className="admin-filter" onChange={(event) => setTo(event.target.value)} type="datetime-local" value={to} />
      <button className="quiet-button" onClick={() => load()} type="button">筛选</button>
    </AdminFilters>
    {error && <div className="admin-inline-error" role="alert"><ShieldAlert aria-hidden="true" size={15} />{error}</div>}
    {items === null ? <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在加载…</strong></div>
      : items.length === 0 ? <div className="empty-state"><FileText aria-hidden="true" size={26} /><strong>暂无匹配调用</strong></div>
        : <><div className="admin-table-wrap"><table><thead><tr><th>请求 ID</th><th>Captcha ID</th><th>Key</th><th>状态</th><th>耗时</th><th>错误摘要</th><th>时间</th></tr></thead><tbody>
          {items.map((item) => <tr key={item.request_id}><td><span className="admin-mono">{item.request_id}</span></td><td><span className="admin-mono">{item.captcha_id}</span></td><td><span className="admin-mono">{item.api_key_prefix}</span></td><td><AdminStatus tone={item.status === 'SUCCEEDED' ? 'success' : item.status === 'FAILED_REFUNDED' ? 'danger' : item.status === 'REJECTED' ? 'warning' : 'neutral'}>{item.status}</AdminStatus></td><td className="admin-mono">{item.duration_ms == null ? '—' : `${item.duration_ms} ms`}</td><td title={item.error_summary ?? ''}>{item.error_summary ?? item.error_code ?? '—'}</td><td>{formatTime(item.accepted_at)}</td></tr>)}
        </tbody></table></div><PageControls loading={loadingMore} nextCursor={nextCursor} onNext={() => { if (nextCursor) { setLoadingMore(true); load(nextCursor, true); } }} /></>}
  </section>;
}

function QuotaLedgerSection() {
  const [items, setItems] = useState<AdminQuotaLedgerEntry[] | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [cdkID, setCdkID] = useState('');
  const [userID, setUserID] = useState('');
  const [entryType, setEntryType] = useState('');
  const [requestID, setRequestID] = useState('');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const load = (cursor?: string, append = false) => {
    setError(null);
    void adminApi.listQuotaLedger({ cdk_id: cdkID, user_id: userID, entry_type: entryType, request_id: requestID, from: asUTCQueryTime(from), to: asUTCQueryTime(to), cursor })
      .then((envelope) => {
        const data = envelope.data ?? { items: [], next_cursor: null };
        setItems((current) => append ? [...(current ?? []), ...data.items] : data.items);
        setNextCursor(data.next_cursor);
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : '网络错误。'))
      .finally(() => setLoadingMore(false));
  };
  // Filters are applied explicitly to avoid a request per keystroke.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { load(); }, []);
  if (error && items === null) return <AdminError message={error} onRetry={load} />;
  return <section className="admin-panel">
    <div className="admin-panel__heading"><div><h2>配额流水</h2><p>只读账本，记录预扣、确认、退款和管理员调整的每次变动。</p></div></div>
    <AdminFilters>
      <select aria-label="流水类型" className="admin-filter" onChange={(event) => setEntryType(event.target.value)} value={entryType}><option value="">全部类型</option><option value="RESERVE">预扣</option><option value="CONFIRM">确认</option><option value="REFUND">退款</option><option value="ADMIN_ADJUSTMENT">管理员调整</option></select>
      <input aria-label="关联 CDK ID" className="admin-filter" onChange={(event) => setCdkID(event.target.value)} placeholder="CDK UUID" value={cdkID} />
      <input aria-label="关联用户 ID" className="admin-filter" onChange={(event) => setUserID(event.target.value)} placeholder="用户 UUID" value={userID} />
      <input aria-label="关联请求 ID" className="admin-filter" onChange={(event) => setRequestID(event.target.value)} placeholder="关联请求 ID" value={requestID} />
      <input aria-label="流水开始时间" className="admin-filter" onChange={(event) => setFrom(event.target.value)} type="datetime-local" value={from} />
      <input aria-label="流水结束时间" className="admin-filter" onChange={(event) => setTo(event.target.value)} type="datetime-local" value={to} />
      <button className="quiet-button" onClick={() => load()} type="button">筛选</button>
    </AdminFilters>
    {error && <div className="admin-inline-error" role="alert"><ShieldAlert aria-hidden="true" size={15} />{error}</div>}
    {items === null ? <div className="empty-state"><RefreshCw aria-hidden="true" size={24} /><strong>正在加载…</strong></div>
      : items.length === 0 ? <div className="empty-state"><Database aria-hidden="true" size={26} /><strong>暂无匹配流水</strong></div>
        : <><div className="admin-table-wrap"><table><thead><tr><th>类型</th><th>额度变动</th><th>可用额度</th><th>已用额度</th><th>原因</th><th>请求 ID</th><th>时间</th></tr></thead><tbody>
          {items.map((item) => <tr key={item.id}><td><AdminStatus tone={item.entry_type === 'REFUND' ? 'warning' : item.entry_type === 'ADMIN_ADJUSTMENT' ? 'neutral' : 'success'}>{item.entry_type}</AdminStatus></td><td className="admin-mono">{item.delta_available > 0 ? `+${item.delta_available}` : item.delta_available}</td><td className="admin-mono">{item.available_before} → {item.available_after}</td><td className="admin-mono">{item.used_before} → {item.used_after}</td><td title={item.reason}>{item.reason}</td><td><span className="admin-mono">{item.request_id}</span></td><td>{formatTime(item.created_at)}</td></tr>)}
        </tbody></table></div><PageControls loading={loadingMore} nextCursor={nextCursor} onNext={() => { if (nextCursor) { setLoadingMore(true); load(nextCursor, true); } }} /></>}
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
  // App owns the history router; keeping a second route state here would
  // leave the URL and visible section out of sync after sidebar navigation.
  const section: AdminSection = initialSection ?? 'overview';
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
          {section === 'cdks' && <CdksSection />}
          {section === 'api-keys' && <ApiKeysSection />}
          {section === 'calls' && <CallsSection />}
          {section === 'quota-ledger' && <QuotaLedgerSection />}
          {section === 'audit' && <AuditSection />}
          {section === 'health' && <HealthSection />}
          {section === 'system' && <SystemSection />}
        </div>
      </main>
    </div>
  );
}
