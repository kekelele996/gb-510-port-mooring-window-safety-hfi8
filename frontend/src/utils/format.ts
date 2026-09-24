
export function formatDate(value: string): string {
  return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-';
}
export function nextStatus(current: string, statuses: readonly string[]): string | null {
  const index = statuses.indexOf(current);
  return index >= 0 && index < statuses.length - 1 ? statuses[index + 1] : null;
}
export function statusTone(status: string): 'success' | 'warning' | 'danger' | 'neutral' {
  if (/approved|accepted|released|completed|signed|closed|pass|ready|online|cleared|succeeded/.test(status)) return 'success';
  if (/failed|rejected|critical|scrap|discard|revoked|urgent/.test(status)) return 'danger';
  if (/hold|warning|review|pending|restricted|limited|quarantine/.test(status)) return 'warning';
  return 'neutral';
}

export const WINDOW_STATUS_LABELS: Record<string, string> = {
  forecast: '预报中',
  safe: '安全',
  restricted: '受限',
  expired: '已过期',
};

export const PLAN_IMPACT_LABELS: Record<string, string> = {
  ready: '可继续执行',
  review: '按方案复核',
  hold: '暂缓执行',
  suspend: '暂停并等待新窗口',
  watch: '等待安全窗口',
};

export const CLEARANCE_IMPACT_LABELS: Record<string, string> = {
  expired: '已同步过期',
  rebind: '已换绑新窗口，待重新确认',
  blocked: '窗口不安全，暂停放行',
  ready: '当前版本有效，可作业',
};

export function impactTone(impact: string): 'success' | 'warning' | 'danger' | 'neutral' {
  if (impact === 'ready') return 'success';
  if (impact === 'expired' || impact === 'suspend' || impact === 'blocked') return 'danger';
  if (impact === 'rebind' || impact === 'hold' || impact === 'review') return 'warning';
  return 'neutral';
}
