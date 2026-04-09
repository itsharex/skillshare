import { useState, useEffect, useRef, useCallback, useMemo, useDeferredValue } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  ArrowUpCircle, RefreshCw, Search, Check, Zap, Trash2,
  Circle, CheckCircle, XCircle, MinusCircle, ShieldAlert, Loader2,
  LayoutGrid, Users, Globe,
} from 'lucide-react';
import { Virtuoso } from 'react-virtuoso';
import Card from '../components/Card';
import Button from '../components/Button';
import SplitButton from '../components/SplitButton';
import PageHeader from '../components/PageHeader';
import EmptyState from '../components/EmptyState';
import Badge from '../components/Badge';
import SegmentedControl from '../components/SegmentedControl';
import StreamProgressBar from '../components/StreamProgressBar';
import KindBadge from '../components/KindBadge';
import SourceBadge from '../components/SourceBadge';
import { PageSkeleton } from '../components/Skeleton';
import { Input } from '../components/Input';
import { Checkbox } from '../components/Checkbox';
import { useToast } from '../components/Toast';
import { api } from '../api/client';
import type { CheckResult } from '../api/client';
import { queryKeys, staleTimes } from '../lib/queryKeys';
import { clearAuditCache } from '../lib/auditCache';
import { radius } from '../design';

/* ── Types ──────────────────────────────────────────── */

type UpdatePhase = 'selecting' | 'updating' | 'done';

type CheckItemStatus = 'unchecked' | 'checking' | 'behind' | 'up-to-date' | 'update-available' | 'error';

type TypeFilter = 'all' | 'tracked' | 'github';

interface UpdatableItem {
  name: string;
  flatName: string;
  kind: 'skill' | 'agent';
  isInRepo: boolean;
  source?: string;
  type?: string;
  relPath: string;
  installedAt?: string;
}

interface ItemUpdateStatus {
  name: string;
  kind?: 'skill' | 'agent';
  isRepo: boolean;
  status: 'pending' | 'in-progress' | 'success' | 'error' | 'blocked' | 'skipped';
  message?: string;
  auditRiskLabel?: string;
}

/* ── Component ──────────────────────────────────────── */

export default function UpdatePage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  // Phase state machine
  const [phase, setPhase] = useState<UpdatePhase>('selecting');

  // Data: skills list
  const { data: skillsData, isPending } = useQuery({
    queryKey: queryKeys.skills.all,
    queryFn: () => api.listSkills(),
    staleTime: staleTimes.skills,
  });
  const allSkills = skillsData?.resources ?? [];

  // Derive updatable items: tracked repos + GitHub-installed skills
  const updatableItems: UpdatableItem[] = useMemo(
    () =>
      allSkills
        .filter((s) => s.isInRepo || s.source)
        .map((s) => ({
          name: s.name,
          flatName: s.flatName,
          kind: s.kind,
          isInRepo: s.isInRepo,
          source: s.source,
          type: s.type,
          relPath: s.relPath,
          installedAt: s.installedAt,
        }))
        .sort((a, b) => a.name.localeCompare(b.name)),
    [allSkills],
  );

  // Check state
  const [checkStatuses, setCheckStatuses] = useState<Map<string, CheckItemStatus>>(new Map());
  const [checking, setChecking] = useState(false);
  const [checkProgress, setCheckProgress] = useState<{ checked: number; total: number } | null>(null);
  const esRef = useRef<EventSource | null>(null);
  const startTimeRef = useRef<number>(0);

  // Selection state
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // Filter state
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState<TypeFilter>('all');
  const deferredSearch = useDeferredValue(search);

  // Updating state
  const [itemStatuses, setItemStatuses] = useState<ItemUpdateStatus[]>([]);
  const itemRefs = useRef<Record<string, HTMLDivElement | null>>({});

  // Clean up EventSource on unmount
  useEffect(() => {
    return () => { esRef.current?.close(); };
  }, []);

  // Auto-scroll to the item currently being updated
  useEffect(() => {
    if (phase !== 'updating') return;
    const inProgressItem = itemStatuses.find((s) => s.status === 'in-progress');
    if (inProgressItem) {
      const el = itemRefs.current[inProgressItem.name];
      if (el) {
        el.scrollIntoView({ behavior: 'smooth', block: 'center' });
      }
    }
  }, [itemStatuses, phase]);

  /* ── Check logic ─────────────────────────────────── */

  const applyCheckResult = useCallback((result: CheckResult, filterNames?: Set<string>) => {
    setCheckStatuses((prev) => {
      const next = new Map(prev);
      for (const repo of result.tracked_repos) {
        if (filterNames && !filterNames.has(repo.name)) continue;
        next.set(repo.name, repo.status === 'behind' ? 'behind' : 'up-to-date');
      }
      for (const skill of result.skills) {
        if (filterNames && !filterNames.has(skill.name)) continue;
        next.set(skill.name, skill.status === 'update_available' ? 'update-available' : 'up-to-date');
      }
      return next;
    });
  }, []);

  const runCheck = useCallback((filterNames?: Set<string>) => {
    esRef.current?.close();
    setChecking(true);
    setCheckProgress(null);
    startTimeRef.current = Date.now();

    // Mark items as checking
    setCheckStatuses((prev) => {
      const next = new Map(prev);
      if (filterNames) {
        for (const name of filterNames) next.set(name, 'checking');
      } else {
        for (const item of updatableItems) next.set(item.name, 'checking');
      }
      return next;
    });

    esRef.current = api.checkStream(
      () => setCheckProgress({ checked: 0, total: 0 }),
      (total) => setCheckProgress({ checked: 0, total }),
      (checked) => setCheckProgress((p) => p ? { ...p, checked } : null),
      (result) => {
        applyCheckResult(result, filterNames);
        setChecking(false);
        setCheckProgress(null);
      },
      (err) => {
        toast(err.message, 'error');
        // Mark items as error
        setCheckStatuses((prev) => {
          const next = new Map(prev);
          if (filterNames) {
            for (const name of filterNames) next.set(name, 'error');
          } else {
            for (const item of updatableItems) next.set(item.name, 'error');
          }
          return next;
        });
        setChecking(false);
        setCheckProgress(null);
      },
    );
  }, [updatableItems, applyCheckResult, toast]);

  const handleCheckAll = useCallback(() => runCheck(), [runCheck]);
  const handleCheckSelected = useCallback(() => {
    if (selected.size === 0) return;
    runCheck(new Set(selected));
  }, [runCheck, selected]);

  /* ── Filtering ───────────────────────────────────── */

  const filterCounts = useMemo(() => {
    const counts: Record<TypeFilter, number> = { all: updatableItems.length, tracked: 0, github: 0 };
    for (const item of updatableItems) {
      if (item.isInRepo) counts.tracked++;
      if ((item.type === 'github' || item.type === 'github-subdir') && !item.isInRepo) counts.github++;
    }
    return counts;
  }, [updatableItems]);

  const filtered = useMemo(() => {
    let list = updatableItems;
    if (deferredSearch.trim()) {
      const lower = deferredSearch.toLowerCase();
      list = list.filter(
        (s) => s.name.toLowerCase().includes(lower) || s.relPath.toLowerCase().includes(lower),
      );
    }
    if (typeFilter === 'tracked') {
      list = list.filter((s) => s.isInRepo);
    } else if (typeFilter === 'github') {
      list = list.filter((s) => (s.type === 'github' || s.type === 'github-subdir') && !s.isInRepo);
    }
    return list;
  }, [updatableItems, deferredSearch, typeFilter]);

  /* ── Selection ───────────────────────────────────── */

  const toggleSelect = useCallback((name: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      next.has(name) ? next.delete(name) : next.add(name);
      return next;
    });
  }, []);

  const allInViewSelected = filtered.length > 0 && filtered.every((s) => selected.has(s.name));

  const selectAll = useCallback(() => {
    setSelected((prev) => {
      const next = new Set(prev);
      filtered.forEach((s) => next.add(s.name));
      return next;
    });
  }, [filtered]);

  const deselectAll = useCallback(() => {
    setSelected((prev) => {
      const next = new Set(prev);
      filtered.forEach((s) => next.delete(s.name));
      return next;
    });
  }, [filtered]);

  /* ── Update logic ────────────────────────────────── */

  const invalidateSkillData = useCallback(() => {
    clearAuditCache(queryClient);
    queryClient.invalidateQueries({ queryKey: queryKeys.overview });
    queryClient.invalidateQueries({ queryKey: queryKeys.skills.all });
  }, [queryClient]);

  const handleUpdate = useCallback((opts?: { force?: boolean }) => {
    if (selected.size === 0) return;

    // Repo deduplication: multiple skills in same repo -> one update
    const seenRepos = new Set<string>();
    const names: string[] = [];
    const items: ItemUpdateStatus[] = [];

    for (const name of selected) {
      const item = updatableItems.find((i) => i.name === name);
      if (!item) continue;

      if (item.isInRepo) {
        const repoDir = item.relPath.split('/')[0];
        if (seenRepos.has(repoDir)) continue;
        seenRepos.add(repoDir);
        names.push(repoDir);
        items.push({ name: repoDir, kind: item.kind, isRepo: true, status: 'pending' });
      } else {
        names.push(item.flatName);
        items.push({ name: item.flatName, kind: item.kind, isRepo: false, status: 'pending' });
      }
    }

    setItemStatuses(items);
    startTimeRef.current = Date.now();
    setPhase('updating');

    let resultIndex = 0;

    api.updateAllStream(
      () => {
        setItemStatuses((prev) =>
          prev.map((s, idx) => (idx === 0 ? { ...s, status: 'in-progress' } : s)),
        );
      },
      (item) => {
        const i = resultIndex;
        resultIndex++;
        setItemStatuses((prev) =>
          prev.map((s, idx) => {
            if (idx === i) {
              return {
                ...s,
                kind: item.kind,
                status: actionToStatus(item.action),
                message: item.message,
                auditRiskLabel: item.auditRiskLabel,
              };
            }
            if (idx === i + 1) {
              return { ...s, status: 'in-progress' };
            }
            return s;
          }),
        );
      },
      () => {
        setPhase('done');
        invalidateSkillData();
      },
      (err) => {
        toast(err.message, 'error');
        setPhase('done');
      },
      { names, force: opts?.force ?? false },
    );
  }, [selected, updatableItems, invalidateSkillData, toast]);

  const patchItem = useCallback(
    (name: string, patch: Partial<ItemUpdateStatus>) =>
      setItemStatuses((prev) => prev.map((s) => (s.name === name ? { ...s, ...patch } : s))),
    [],
  );

  const handleRetryForce = useCallback(
    (name: string) => {
      patchItem(name, { status: 'in-progress', message: undefined });
      api.updateAllStream(
        () => {},
        (item) => {
          patchItem(name, {
            status: actionToStatus(item.action),
            message: item.message,
            auditRiskLabel: item.auditRiskLabel,
          });
        },
        () => invalidateSkillData(),
        (err) => patchItem(name, { status: 'error', message: err.message }),
        { names: [name], force: true },
      );
    },
    [patchItem, invalidateSkillData],
  );

  const handlePurge = useCallback(
    async (name: string) => {
      patchItem(name, { status: 'in-progress', message: 'Purging...' });
      try {
        await api.batchUninstall({ names: [name], force: true });
        patchItem(name, { status: 'skipped', message: 'Purged' });
        invalidateSkillData();
      } catch (err) {
        patchItem(name, { status: 'error', message: (err as Error).message });
      }
    },
    [patchItem, invalidateSkillData],
  );

  const handleBackToList = useCallback(() => {
    setPhase('selecting');
    setItemStatuses([]);
    setSelected(new Set());
    setCheckStatuses(new Map());
    itemRefs.current = {};
  }, []);

  /* ── Derived counts ──────────────────────────────── */

  const successCount = itemStatuses.filter((s) => s.status === 'success').length;
  const skippedCount = itemStatuses.filter((s) => s.status === 'skipped').length;
  const blockedCount = itemStatuses.filter((s) => s.status === 'blocked').length;
  const errorCount = itemStatuses.filter((s) => s.status === 'error').length;
  const completedCount = successCount + skippedCount + blockedCount + errorCount;

  /* ── Render ──────────────────────────────────────── */

  if (isPending) return <PageSkeleton />;

  // ── Selecting phase ──────────────────────────────

  if (phase === 'selecting') {
    const typeFilterOptions = [
      {
        value: 'all' as TypeFilter,
        label: <span className="inline-flex items-center gap-1.5"><LayoutGrid size={14} strokeWidth={2.5} />All</span>,
        count: filterCounts.all,
      },
      {
        value: 'tracked' as TypeFilter,
        label: <span className="inline-flex items-center gap-1.5"><Users size={14} strokeWidth={2.5} />Tracked</span>,
        count: filterCounts.tracked,
      },
      {
        value: 'github' as TypeFilter,
        label: <span className="inline-flex items-center gap-1.5"><Globe size={14} strokeWidth={2.5} />GitHub</span>,
        count: filterCounts.github,
      },
    ];

    return (
      <div className="space-y-3 animate-fade-in">
        <PageHeader
          icon={<ArrowUpCircle size={24} strokeWidth={2.5} />}
          title="Updates"
          subtitle="Check and apply updates for tracked repositories and installed skills."
          actions={
            <>
              <Button
                variant="ghost"
                size="sm"
                onClick={handleCheckAll}
                loading={checking && selected.size === 0}
                disabled={checking || updatableItems.length === 0}
              >
                <RefreshCw size={16} />
                Check All
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={handleCheckSelected}
                loading={checking && selected.size > 0}
                disabled={checking || selected.size === 0}
              >
                <RefreshCw size={16} />
                Check Selected
              </Button>
              <SplitButton
                variant="primary"
                size="sm"
                onClick={() => handleUpdate()}
                disabled={selected.size === 0}
                dropdownAlign="right"
                items={[
                  {
                    label: 'Force Update',
                    icon: <Zap size={14} strokeWidth={2.5} />,
                    onClick: () => handleUpdate({ force: true }),
                    confirm: true,
                  },
                ]}
              >
                <ArrowUpCircle size={16} />
                Update Selected ({selected.size})
              </SplitButton>
            </>
          }
        />

        {/* Check progress bar */}
        {checking && (
          <StreamProgressBar
            count={checkProgress?.checked ?? 0}
            total={checkProgress?.total ?? 0}
            startTime={startTimeRef.current}
            icon={RefreshCw}
            labelDiscovering="Discovering skills..."
            labelRunning="Checking for updates..."
            units="sources"
          />
        )}

        {updatableItems.length === 0 ? (
          <EmptyState
            icon={Check}
            title="No updatable skills"
            description="Install tracked repositories or GitHub skills to check for updates."
          />
        ) : (
          <>
            {/* Sticky toolbar */}
            <div className="sticky top-0 z-20 bg-paper -mx-4 px-4 md:-mx-8 md:px-8 py-2 mb-1 space-y-2">
              {/* Search */}
              <div className="relative">
                <Search
                  size={18}
                  strokeWidth={2.5}
                  className="absolute left-4 top-1/2 -translate-y-1/2 text-muted-dark pointer-events-none"
                />
                <Input
                  placeholder="Filter skills..."
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  className="!pl-11"
                />
              </div>

              {/* Filters + selection actions */}
              <div className="flex flex-wrap items-center gap-3">
                <SegmentedControl
                  value={typeFilter}
                  onChange={setTypeFilter}
                  size="sm"
                  options={typeFilterOptions}
                />
                <div className="flex items-center gap-2 ml-auto">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={allInViewSelected ? deselectAll : selectAll}
                    disabled={filtered.length === 0}
                  >
                    {allInViewSelected ? 'Deselect All' : 'Select All'}
                  </Button>
                  {selected.size > 0 && (
                    <Button variant="ghost" size="sm" onClick={() => setSelected(new Set())}>
                      Clear
                    </Button>
                  )}
                </div>
              </div>
            </div>

            {/* Virtuoso list */}
            {filtered.length === 0 ? (
              <div className="py-12 text-center">
                <p className="text-pencil-light text-sm">No skills match your filter.</p>
              </div>
            ) : (
              <div
                className="border border-muted bg-surface overflow-hidden"
                style={{ borderRadius: radius.md }}
              >
                <Virtuoso
                  useWindowScroll
                  totalCount={filtered.length}
                  overscan={200}
                  itemContent={(index) => {
                    const item = filtered[index];
                    const isSelected = selected.has(item.name);
                    const checkStatus = checkStatuses.get(item.name) ?? 'unchecked';
                    return (
                      <button
                        type="button"
                        className={`
                          w-full text-left px-4 py-2.5 flex items-center gap-3
                          transition-colors duration-100 cursor-pointer
                          ${index > 0 ? 'border-t border-muted/40' : ''}
                          ${isSelected ? 'bg-blue/5' : 'hover:bg-muted/15'}
                        `}
                        onClick={() => toggleSelect(item.name)}
                      >
                        <Checkbox
                          label=""
                          checked={isSelected}
                          onChange={() => toggleSelect(item.name)}
                          size="sm"
                        />
                        <div className="flex-1 min-w-0">
                          <span className="font-mono text-sm text-pencil truncate flex items-center gap-1.5">
                            <KindBadge kind={item.kind} />
                            {item.name}
                          </span>
                          {item.source && (
                            <span className="text-xs text-pencil-light truncate block">
                              {[item.source, item.installedAt && formatRelativeTime(item.installedAt)]
                                .filter(Boolean)
                                .join(' · ')}
                            </span>
                          )}
                        </div>
                        <div className="flex items-center gap-1.5 shrink-0">
                          <SourceBadge type={item.type} isInRepo={item.isInRepo} />
                          <CheckStatusBadge status={checkStatus} />
                        </div>
                      </button>
                    );
                  }}
                />
              </div>
            )}
          </>
        )}
      </div>
    );
  }

  // ── Updating phase ───────────────────────────────

  if (phase === 'updating') {
    return (
      <div className="space-y-4 animate-fade-in">
        <PageHeader
          icon={<ArrowUpCircle size={24} strokeWidth={2.5} />}
          title="Updates"
          subtitle="Updating selected skills..."
        />

        {/* Sticky progress bar */}
        <div className="sticky top-0 z-20 bg-paper -mx-4 px-4 md:-mx-8 md:px-8 pt-2 pb-3">
          <StreamProgressBar
            count={completedCount}
            total={itemStatuses.length}
            startTime={startTimeRef.current}
            icon={ArrowUpCircle}
            iconClassName=""
            labelDiscovering="Starting update..."
            labelRunning="Updating..."
            units="items"
          />
        </div>

        {/* Per-item status cards */}
        <div className="space-y-2">
          {itemStatuses.map((item, i) => (
            <div
              key={item.name}
              ref={(el) => { itemRefs.current[item.name] = el; }}
              className={`flex items-center gap-3 px-3 py-2 border transition-colors animate-fade-in ${
                item.status === 'error'
                  ? 'border-danger/40 bg-danger-light/50'
                  : item.status === 'blocked'
                  ? 'border-warning/40 bg-warning-light/50'
                  : 'border-muted hover:bg-muted/30'
              }`}
              style={{
                borderRadius: radius.sm,
                animationDelay: `${i * 50}ms`,
                animationFillMode: 'backwards',
              }}
            >
              <StatusIcon status={item.status} />
              <div className="flex-1 min-w-0">
                <span className="text-pencil font-medium flex items-center gap-1.5">
                  {item.kind && <KindBadge kind={item.kind} />}
                  {item.name}
                </span>
                {item.message && (
                  <span
                    className={`text-sm block ${
                      item.status === 'error'
                        ? 'text-danger font-medium whitespace-pre-wrap mt-1'
                        : item.status === 'blocked'
                        ? 'text-warning whitespace-pre-wrap mt-1'
                        : 'text-pencil-light truncate'
                    }`}
                  >
                    {item.message}
                  </span>
                )}
              </div>
              <div className="flex items-center gap-2 shrink-0">
                {item.auditRiskLabel && item.auditRiskLabel !== 'clean' && (
                  <Badge variant={item.auditRiskLabel === 'critical' || item.auditRiskLabel === 'high' ? 'danger' : 'warning'}>
                    <ShieldAlert size={12} className="mr-1" />
                    {item.auditRiskLabel}
                  </Badge>
                )}
                <StatusBadge status={item.status} />
              </div>
            </div>
          ))}
        </div>
      </div>
    );
  }

  // ── Done phase ───────────────────────────────────

  return (
    <div className="space-y-4 animate-fade-in">
      <PageHeader
        icon={<ArrowUpCircle size={24} strokeWidth={2.5} />}
        title="Updates"
        subtitle="Update complete."
        actions={
          <Button variant="ghost" size="sm" onClick={handleBackToList}>
            Back to List
          </Button>
        }
      />

      {/* Summary card */}
      <Card tilt>
        <div className="flex items-center justify-between flex-wrap gap-3">
          <div className="flex items-center gap-2 flex-wrap">
            {successCount > 0 && <Badge variant="success">{successCount} updated</Badge>}
            {skippedCount > 0 && <Badge>{skippedCount} skipped</Badge>}
            {blockedCount > 0 && <Badge variant="warning">{blockedCount} blocked</Badge>}
            {errorCount > 0 && <Badge variant="danger">{errorCount} failed</Badge>}
          </div>
        </div>
      </Card>

      {/* Per-item result cards with action buttons */}
      <div className="space-y-2">
        {itemStatuses.map((item, i) => (
          <div
            key={item.name}
            className={`flex items-center gap-3 px-3 py-2 border transition-colors animate-fade-in ${
              item.status === 'error'
                ? 'border-danger/40 bg-danger-light/50'
                : item.status === 'blocked'
                ? 'border-warning/40 bg-warning-light/50'
                : 'border-muted hover:bg-muted/30'
            }`}
            style={{
              borderRadius: radius.sm,
              animationDelay: `${i * 50}ms`,
              animationFillMode: 'backwards',
            }}
          >
            <StatusIcon status={item.status} />
            <div className="flex-1 min-w-0">
              <span className="text-pencil font-medium flex items-center gap-1.5">
                {item.kind && <KindBadge kind={item.kind} />}
                {item.name}
              </span>
              {item.message && (
                <span
                  className={`text-sm block ${
                    item.status === 'error'
                      ? 'text-danger font-medium whitespace-pre-wrap mt-1'
                      : item.status === 'blocked'
                      ? 'text-warning whitespace-pre-wrap mt-1'
                      : 'text-pencil-light truncate'
                  }`}
                >
                  {item.message}
                </span>
              )}
            </div>
            <div className="flex items-center gap-2 shrink-0">
              {item.auditRiskLabel && item.auditRiskLabel !== 'clean' && (
                <Badge variant={item.auditRiskLabel === 'critical' || item.auditRiskLabel === 'high' ? 'danger' : 'warning'}>
                  <ShieldAlert size={12} className="mr-1" />
                  {item.auditRiskLabel}
                </Badge>
              )}
              {item.status === 'error' && (
                isStaleError(item.message) ? (
                  <Button variant="danger" size="sm" onClick={() => handlePurge(item.name)}>
                    <Trash2 size={14} />
                    Purge
                  </Button>
                ) : (
                  <Button variant="danger" size="sm" onClick={() => handleRetryForce(item.name)}>
                    <RefreshCw size={14} />
                    Force Retry
                  </Button>
                )
              )}
              <StatusBadge status={item.status} />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

/* ── Helper functions ──────────────────────────────── */

function formatRelativeTime(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  if (isNaN(then)) return dateStr;
  const diff = Math.floor((now - then) / 1000);
  if (diff < 60) return 'just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  if (diff < 2592000) return `${Math.floor(diff / 86400)}d ago`;
  if (diff < 31536000) return `${Math.floor(diff / 2592000)}mo ago`;
  return `${Math.floor(diff / 31536000)}y ago`;
}

function isStaleError(message?: string): boolean {
  if (!message) return false;
  return message.includes('does not exist in repository') || message.includes('not found in repository');
}

function actionToStatus(action: string): ItemUpdateStatus['status'] {
  switch (action) {
    case 'updated': return 'success';
    case 'error': return 'error';
    case 'blocked': return 'blocked';
    case 'skipped':
    case 'up-to-date': return 'skipped';
    default: return 'success';
  }
}

function StatusIcon({ status }: { status: ItemUpdateStatus['status'] }) {
  switch (status) {
    case 'pending':
      return <Circle size={16} className="text-muted-dark shrink-0" />;
    case 'in-progress':
      return <Loader2 size={16} className="text-blue animate-spin shrink-0" />;
    case 'success':
      return <CheckCircle size={16} className="text-success shrink-0" />;
    case 'error':
      return <XCircle size={16} className="text-danger shrink-0" />;
    case 'blocked':
      return <ShieldAlert size={16} className="text-warning shrink-0" />;
    case 'skipped':
      return <MinusCircle size={16} className="text-muted-dark shrink-0" />;
  }
}

function StatusBadge({ status }: { status: ItemUpdateStatus['status'] }) {
  switch (status) {
    case 'pending':
      return <Badge>Pending</Badge>;
    case 'in-progress':
      return <Badge variant="info">Updating</Badge>;
    case 'success':
      return <Badge variant="success">Updated</Badge>;
    case 'error':
      return <Badge variant="danger">Failed</Badge>;
    case 'blocked':
      return <Badge variant="warning">Blocked</Badge>;
    case 'skipped':
      return <Badge>Skipped</Badge>;
  }
}

function CheckStatusBadge({ status }: { status: CheckItemStatus }) {
  switch (status) {
    case 'unchecked':
      return <Badge size="sm">Unchecked</Badge>;
    case 'checking':
      return <Badge variant="info" size="sm"><Loader2 size={10} className="animate-spin mr-1" />Checking</Badge>;
    case 'behind':
      return <Badge variant="warning" size="sm">Behind</Badge>;
    case 'up-to-date':
      return <Badge variant="success" size="sm">Up to date</Badge>;
    case 'update-available':
      return <Badge variant="warning" size="sm">Update available</Badge>;
    case 'error':
      return <Badge variant="danger" size="sm">Error</Badge>;
  }
}
