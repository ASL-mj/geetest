// Shared presentation helpers for statuses and timestamps.

export type StatusTone = 'success' | 'warning' | 'danger' | 'neutral' | 'info';

export function callStatusMeta(status: string): { label: string; tone: StatusTone } {
  switch (status) {
    case 'SUCCEEDED':
      return { label: '成功', tone: 'success' };
    case 'FAILED_REFUNDED':
      return { label: '失败已退回', tone: 'danger' };
    case 'REJECTED':
      return { label: '已拒绝', tone: 'warning' };
    case 'DISPATCHED':
    case 'RESERVED':
    case 'RECEIVED':
      return { label: '处理中', tone: 'info' };
    default:
      return { label: status, tone: 'neutral' };
  }
}

export function keyStatusMeta(status: string): { label: string; tone: StatusTone } {
  switch (status) {
    case 'ACTIVE':
      return { label: '已启用', tone: 'success' };
    case 'DISABLED':
      return { label: '已禁用', tone: 'warning' };
    case 'DELETED':
      return { label: '已删除', tone: 'neutral' };
    default:
      return { label: status, tone: 'neutral' };
  }
}

export function cdkStatusMeta(status: string): { label: string; tone: StatusTone } {
  switch (status) {
    case 'ACTIVE':
      return { label: '有效', tone: 'success' };
    case 'UNACTIVATED':
      return { label: '未激活', tone: 'info' };
    case 'DISABLED':
      return { label: '已禁用', tone: 'danger' };
    case 'EXPIRED':
      return { label: '已过期', tone: 'neutral' };
    default:
      return { label: status, tone: 'neutral' };
  }
}

const timeFormatter = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
});

export function formatTime(value: string | null | undefined): string {
  if (!value) return '—';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return timeFormatter.format(parsed).replace(/\//g, '-');
}

export function formatCount(value: number): string {
  return value.toLocaleString('zh-CN');
}
