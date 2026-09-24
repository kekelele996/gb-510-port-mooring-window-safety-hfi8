
export interface DomainRecord {
  id: number;
  code: string;
  name: string;
  status: string;
  version: number;
  description: string;
  facility: string;
  owner: string;
  category: string;
  riskLevel: 'low' | 'medium' | 'high' | 'critical';
  metricValue: number;
  metricUnit: string;
  effectiveAt: string;
  evidence: string;
  relatedCode: string;
  windowCode?: string;
  windowVersion?: number;
  rebindRequired?: boolean;
  submittedBy?: string;
  submittedAt?: string;
  confirmedBy?: string;
  confirmedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface PageMeta { page: number; pageSize: number; total: number }
export interface ApiEnvelope<T> { data: T; error?: string; message?: string; meta?: PageMeta }
export interface UserSession { token: string; username: string; displayName: string; role: string; expiresIn: number }
export interface AuditLog {
  id: number; requestId: string; actor: string; action: string; entityType: string;
  entityId: number; beforeState: string; afterState: string; windowVersion?: number; detail: string; createdAt: string;
}
export interface EntityConfig { key: string; path: string; label: string; statuses: readonly string[] }

export type PlanImpactState = 'ready' | 'review' | 'hold' | 'suspend' | 'watch';
export type ClearanceImpactState = 'expired' | 'rebind' | 'blocked' | 'ready';

export interface PlanImpact extends DomainRecord { impact: PlanImpactState }
export interface ClearanceImpact extends DomainRecord { impact: ClearanceImpactState }

export interface WindowImpactAssessment {
  window: DomainRecord;
  plans: PlanImpact[];
  clearances: ClearanceImpact[];
  expiredClearances: number;
  reboundClearances: number;
  blockedClearances: number;
  readyClearances: number;
  decision: 'continue' | 'suspend';
  decisionReason: string;
  assessedAt: string;
}
