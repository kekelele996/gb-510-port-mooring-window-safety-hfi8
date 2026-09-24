
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
  windowVersion?: number;
  windowStatus?: string;
  windowCurrentVersion?: number;
  windowLinked?: boolean;
  submittedBy?: string;
  submittedAt?: string;
  confirmedBy?: string;
  confirmedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface PageMeta { page: number; pageSize: number; total: number }
export interface ApiEnvelope<T> { data: T; error?: string; message?: string; meta?: PageMeta }

export interface ImpactedPlan {
  id: number; code: string; name: string; status: string; riskLevel: string;
  owner: string; version: number; recommendation: string;
}
export interface ImpactedClearance {
  id: number; code: string; name: string; status: string; riskLevel: string;
  owner: string; version: number; windowVersion: number;
  submittedBy: string; confirmedBy: string; staleVersion: boolean; recommendation: string;
}
export interface WindowImpact {
  window: DomainRecord;
  decision: 'continue' | 'suspend';
  decisionReason: string;
  planCount: number;
  clearanceCount: number;
  expiredCount: number;
  staleCount: number;
  plans: ImpactedPlan[];
  clearances: ImpactedClearance[];
}
export interface UserSession { token: string; username: string; displayName: string; role: string; expiresIn: number }
export interface AuditLog {
  id: number; requestId: string; actor: string; action: string; entityType: string;
  entityId: number; beforeState: string; afterState: string; windowVersion?: number; detail: string; createdAt: string;
}
export interface EntityConfig { key: string; path: string; label: string; statuses: readonly string[] }
