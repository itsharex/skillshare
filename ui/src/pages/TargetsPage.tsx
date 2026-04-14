import { useState, useMemo } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Trash2, Plus, Target, ArrowDownToLine, Search, CircleDot, PenLine, AlertTriangle, X } from 'lucide-react';
import Card from '../components/Card';
import StatusBadge from '../components/StatusBadge';
import Button from '../components/Button';
import IconButton from '../components/IconButton';
import { Input, Select } from '../components/Input';
import EmptyState from '../components/EmptyState';
import ConfirmDialog from '../components/ConfirmDialog';
import DialogShell from '../components/DialogShell';
import { PageSkeleton } from '../components/Skeleton';
import PageHeader from '../components/PageHeader';
import { useToast } from '../components/Toast';
import { api } from '../api/client';
import type { AvailableTarget, Target as TargetType } from '../api/client';
import { queryKeys, staleTimes } from '../lib/queryKeys';
import { radius, shadows } from '../design';
import { shortenHome } from '../lib/paths';
import { useSyncMatrix } from '../hooks/useSyncMatrix';

const SYNC_MODE_OPTIONS = [
  { value: 'merge', label: 'Merge (default)', description: 'Per-skill symlinks, preserves local skills' },
  { value: 'symlink', label: 'Symlink', description: 'Entire directory symlinked to source' },
  { value: 'copy', label: 'Copy', description: 'Physical file copies instead of symlinks' },
];

const TARGET_NAMING_OPTIONS = [
  { value: 'flat', label: 'Flat (default)', description: 'Flattened names with __ separators' },
  { value: 'standard', label: 'Standard', description: 'SKILL.md name (Agent Skills spec)' },
];

const AGENT_MODE_OPTIONS = [
  { value: 'merge', label: 'Merge (default)', description: 'Per-agent symlinks, preserves local agents' },
  { value: 'symlink', label: 'Symlink', description: 'Entire directory symlinked to source' },
  { value: 'copy', label: 'Copy', description: 'Physical file copies instead of symlinks' },
];

export type CollectScope = 'skill' | 'agent' | 'both';

function pluralize(count: number, singular: string): string {
  return `${count} ${singular}${count === 1 ? '' : 's'}`;
}

export function getTargetCollectScope(target: TargetType): CollectScope | null {
  const hasLocalSkills = target.localCount > 0;
  const hasLocalAgents = (target.agentLocalCount ?? 0) > 0;

  if (hasLocalSkills && hasLocalAgents) return 'both';
  if (hasLocalAgents) return 'agent';
  if (hasLocalSkills) return 'skill';
  return null;
}

export function targetAgentSummary(target: TargetType): { text: string; hasDrift: boolean } {
  const expected = target.agentExpectedCount ?? 0;
  const linked = target.agentLinkedCount ?? 0;
  const local = target.agentLocalCount ?? 0;
  const label = target.agentMode === 'copy' ? 'managed' : 'linked';
  const hasDrift = expected > 0 && linked !== expected;

  if (expected === 0) {
    const counts = [
      linked > 0 ? pluralize(linked, label) : null,
      local > 0 ? pluralize(local, 'local') : null,
    ].filter(Boolean).join(', ');
    if (counts) {
      return { text: `No source agents yet (${counts})`, hasDrift: false };
    }
    return { text: 'No source agents yet', hasDrift: false };
  }

  const suffix = target.agentMode === 'symlink' ? ' (directory symlink)' : '';
  const localSuffix = local > 0 ? `, ${pluralize(local, 'local')}` : '';
  return { text: `${linked}/${expected} ${label}${suffix}${localSuffix}`, hasDrift };
}

export function targetAgentAvailabilityText(target: TargetType): string {
  const expected = target.agentExpectedCount ?? 0;
  const local = target.agentLocalCount ?? 0;
  const agentFilters = (target.agentInclude?.length ?? 0) + (target.agentExclude?.length ?? 0);

  if (expected === 0) {
    return local > 0 ? pluralize(local, 'local agent') : 'No agents';
  }

  if (!agentFilters) return `All ${expected} agents`;
  const linked = target.agentLinkedCount ?? 0;
  return `${linked}/${expected} agents`;
}

export default function TargetsPage() {
  const queryClient = useQueryClient();
  const { data, isPending, error } = useQuery({
    queryKey: queryKeys.targets.all,
    queryFn: () => api.listTargets(),
    staleTime: staleTimes.targets,
  });
  const availTargets = useQuery({
    queryKey: queryKeys.targets.available,
    queryFn: () => api.availableTargets(),
    staleTime: staleTimes.targets,
  });
  const [adding, setAdding] = useState(false);
  const [newTarget, setNewTarget] = useState({ name: '', path: '', agentPath: '' });
  const [searchQuery, setSearchQuery] = useState('');
  const [customMode, setCustomMode] = useState(false);
  const [removing, setRemoving] = useState<string | null>(null);
  const [collecting, setCollecting] = useState<{ name: string; scope: CollectScope } | null>(null);
  const navigate = useNavigate();
  const { getTargetSummary } = useSyncMatrix();
  const { toast } = useToast();

  const updateTargetSetting = async (
    targetName: string,
    payload: Parameters<typeof api.updateTarget>[1],
    label: string,
  ) => {
    // Optimistic update: apply change to cache immediately
    const queryKey = queryKeys.targets.all;
    const previous = queryClient.getQueryData<{ targets: TargetType[]; sourceSkillCount: number }>(queryKey);

    if (previous) {
      queryClient.setQueryData(queryKey, {
        ...previous,
        targets: previous.targets.map((t) =>
          t.name === targetName ? {
            ...t,
            ...payload,
            ...(payload.target_naming != null ? { targetNaming: payload.target_naming } : {}),
            ...(payload.agent_mode != null ? { agentMode: payload.agent_mode } : {}),
          } : t,
        ),
      });
    }

    try {
      await api.updateTarget(targetName, payload);
      queryClient.invalidateQueries({ queryKey });
      queryClient.invalidateQueries({ queryKey: queryKeys.config });
      queryClient.invalidateQueries({ queryKey: queryKeys.diff() });
      toast(`${targetName}: ${label}`, 'success');
    } catch (e) {
      // Rollback on error
      if (previous) queryClient.setQueryData(queryKey, previous);
      toast((e as Error).message, 'error');
    }
  };

  // Compute filtered & sectioned available targets
  const { detected, others } = useMemo(() => {
    const all = (availTargets.data?.targets ?? []).filter((t) => !t.installed);
    const q = searchQuery.toLowerCase().trim();
    const filtered = q ? all.filter((t) => t.name.toLowerCase().includes(q)) : all;
    const sorted = [...filtered].sort((a, b) => a.name.localeCompare(b.name));
    return {
      detected: sorted.filter((t) => t.detected),
      others: sorted.filter((t) => !t.detected),
    };
  }, [availTargets.data, searchQuery]);

  if (isPending) return <PageSkeleton />;
  if (error) {
    return (
      <Card variant="accent" className="text-center py-8">
        <p className="text-danger text-lg">
          Failed to load targets
        </p>
        <p className="text-pencil-light text-sm mt-1">{error.message}</p>
      </Card>
    );
  }

  const targets = data?.targets ?? [];
  const sourceSkillCount = data?.sourceSkillCount ?? 0;

  const handleAdd = async () => {
    if (!newTarget.name) return;
    try {
      const avail = availTargets.data?.targets.find((t) => t.name === newTarget.name);
      const path = newTarget.path || avail?.path || '';
      if (!path) return;
      const agentPath = newTarget.agentPath || avail?.agentPath || '';
      await api.addTarget(newTarget.name, path, agentPath || undefined);
      setAdding(false);
      setNewTarget({ name: '', path: '', agentPath: '' });
      setSearchQuery('');
      setCustomMode(false);
      toast(`Target "${newTarget.name}" added.`, 'success');
      queryClient.invalidateQueries({ queryKey: queryKeys.targets.all });
      queryClient.invalidateQueries({ queryKey: queryKeys.targets.available });
      queryClient.invalidateQueries({ queryKey: queryKeys.config });
      queryClient.invalidateQueries({ queryKey: queryKeys.overview });
      queryClient.invalidateQueries({ queryKey: queryKeys.diff() });
    } catch (e: unknown) {
      toast((e as Error).message, 'error');
    }
  };

  const handleRemove = async (name: string) => {
    try {
      await api.removeTarget(name);
      toast(`Target "${name}" removed.`, 'success');
      setRemoving(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.targets.all });
      queryClient.invalidateQueries({ queryKey: queryKeys.config });
      queryClient.invalidateQueries({ queryKey: queryKeys.overview });
      queryClient.invalidateQueries({ queryKey: queryKeys.diff() });
    } catch (e: unknown) {
      toast((e as Error).message, 'error');
      setRemoving(null);
    }
  };

  return (
    <div className="animate-fade-in">
      {/* Header */}
      <PageHeader
        icon={<Target size={24} strokeWidth={2.5} />}
        title="Targets"
        subtitle={`${targets.length} target${targets.length !== 1 ? 's' : ''} configured`}
        actions={
          <Button
            onClick={() => setAdding(true)}
            variant="primary"
            size="sm"
          >
            <Plus size={16} strokeWidth={2.5} />
            Add Target
          </Button>
        }
      />

      {/* Add target modal */}
      <DialogShell
        open={adding}
        onClose={() => {
          setAdding(false);
          setNewTarget({ name: '', path: '', agentPath: '' });
          setSearchQuery('');
          setCustomMode(false);
        }}
        maxWidth="xl"
        padding="none"
      >
        <div className="p-5 pb-0 flex items-center justify-between">
          <h3 className="font-bold text-pencil text-lg">Add New Target</h3>
          <button
            onClick={() => {
              setAdding(false);
              setNewTarget({ name: '', path: '', agentPath: '' });
              setSearchQuery('');
              setCustomMode(false);
            }}
            className="w-8 h-8 flex items-center justify-center text-pencil-light hover:text-pencil transition-colors cursor-pointer"
            aria-label="Close"
          >
            <X size={18} strokeWidth={2.5} />
          </button>
        </div>

        {/* Selected target preview + path + actions */}
        {newTarget.name && !customMode ? (
          <div className="p-5 space-y-4 animate-fade-in">
            <div
              className="flex items-center gap-3 bg-surface border-2 border-blue px-4 py-3"
              style={{ borderRadius: radius.sm, boxShadow: shadows.sm }}
            >
              <Target size={18} strokeWidth={2.5} className="text-blue shrink-0" />
              <div className="min-w-0 flex-1">
                <p className="font-bold text-pencil">
                  {newTarget.name}
                </p>
                {newTarget.agentPath ? (
                  <>
                    <p className="font-mono text-sm text-pencil-light truncate">
                      <span className="text-[10px] font-bold uppercase tracking-wider text-pencil-light/70 mr-1.5">Skills</span>
                      {shortenHome(newTarget.path)}
                    </p>
                    <p className="font-mono text-sm text-pencil-light truncate">
                      <span className="text-[10px] font-bold uppercase tracking-wider text-pencil-light/70 mr-1.5">Agents</span>
                      {shortenHome(newTarget.agentPath)}
                    </p>
                  </>
                ) : (
                  <p className="font-mono text-sm text-pencil-light truncate">
                    {shortenHome(newTarget.path)}
                  </p>
                )}
              </div>
            </div>

            <Input
              label={newTarget.agentPath ? 'Skills path (customize if needed)' : 'Path (customize if needed)'}
              type="text"
              value={newTarget.path}
              onChange={(e) => setNewTarget({ ...newTarget, path: e.target.value })}
              placeholder="/path/to/target/skills"
            />
            {newTarget.agentPath && (
              <Input
                label="Agents path (customize if needed)"
                type="text"
                value={newTarget.agentPath}
                onChange={(e) => setNewTarget({ ...newTarget, agentPath: e.target.value })}
                placeholder="/path/to/target/agents"
              />
            )}

            <div className="flex items-center justify-between pt-2">
              <Button
                onClick={() => setNewTarget({ name: '', path: '', agentPath: '' })}
                variant="ghost"
                size="sm"
              >
                Change
              </Button>
              <Button onClick={handleAdd} variant="primary" size="sm">
                <Plus size={16} strokeWidth={2.5} />
                Add Target
              </Button>
            </div>
          </div>
        ) : customMode ? (
          /* Custom target entry mode */
          <div className="p-5 space-y-4 animate-fade-in">
            <Input
              label="Target Name"
              type="text"
              value={newTarget.name}
              onChange={(e) => setNewTarget({ ...newTarget, name: e.target.value })}
              placeholder="my-custom-target"
              autoFocus
            />
            <Input
              label="Skills Path"
              type="text"
              value={newTarget.path}
              onChange={(e) => setNewTarget({ ...newTarget, path: e.target.value })}
              placeholder="/path/to/target/skills"
            />
            <Input
              label="Agents Path (optional)"
              type="text"
              value={newTarget.agentPath}
              onChange={(e) => setNewTarget({ ...newTarget, agentPath: e.target.value })}
              placeholder="/path/to/target/agents"
            />
            <div className="flex items-center justify-between pt-2">
              <Button
                onClick={() => {
                  setCustomMode(false);
                  setNewTarget({ name: '', path: '', agentPath: '' });
                }}
                variant="ghost"
                size="sm"
              >
                Back to picker
              </Button>
              <Button onClick={handleAdd} variant="primary" size="sm">
                <Plus size={16} strokeWidth={2.5} />
                Add Target
              </Button>
            </div>
          </div>
        ) : (
          /* Target picker mode */
          <div className="p-5 pt-4 space-y-3">
            {/* Search bar */}
            <div className="relative">
              <Search
                size={18}
                strokeWidth={2.5}
                className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-dark pointer-events-none"
              />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search targets..."
                className="w-full pl-10 pr-4 py-2.5 bg-surface border-2 border-muted text-pencil placeholder:text-muted-dark focus:outline-none focus:border-pencil transition-all"
                style={{
                  borderRadius: radius.sm,
                  fontSize: '1rem',
                }}
                autoFocus
              />
            </div>

            {/* Scrollable target list */}
            <div
              className="max-h-[60vh] overflow-y-auto border-2 border-dashed border-muted-dark bg-surface"
              style={{ borderRadius: radius.md }}
            >
              {/* Detected section */}
              {detected.length > 0 && (
                <div>
                  <div className="px-3 py-2 border-b border-dashed border-muted-dark sticky top-0 z-10 bg-surface relative">
                    <div className="absolute inset-0 bg-success-light pointer-events-none" />
                    <span className="relative text-sm font-bold text-success flex items-center gap-1.5">
                      <CircleDot size={14} strokeWidth={3} />
                      Detected on your system
                    </span>
                  </div>
                  {detected.map((t) => (
                    <TargetPickerItem
                      key={t.name}
                      target={t}
                      isDetected
                      onSelect={(target) => {
                        setNewTarget({ name: target.name, path: target.path, agentPath: target.agentPath || '' });
                        setSearchQuery('');
                      }}
                    />
                  ))}
                </div>
              )}

              {/* All available section */}
              {others.length > 0 && (
                <div>
                  <div className="px-3 py-2 border-b border-dashed border-muted-dark sticky top-0 z-10 bg-surface">
                    <span className="text-sm font-bold text-pencil-light">
                      All available targets
                    </span>
                  </div>
                  {others.map((t) => (
                    <TargetPickerItem
                      key={t.name}
                      target={t}
                      onSelect={(target) => {
                        setNewTarget({ name: target.name, path: target.path, agentPath: target.agentPath || '' });
                        setSearchQuery('');
                      }}
                    />
                  ))}
                </div>
              )}

              {/* No results */}
              {detected.length === 0 && others.length === 0 && (
                <div className="px-4 py-8 text-center text-pencil-light">
                  {searchQuery ? `No targets matching "${searchQuery}"` : 'No available targets'}
                </div>
              )}
            </div>

            {/* Custom target link */}
            <div className="flex items-center justify-between">
              <Button
                variant="link"
                onClick={() => setCustomMode(true)}
                className="inline-flex items-center gap-1.5"
              >
                <PenLine size={14} strokeWidth={2.5} />
                Enter custom target
              </Button>
            </div>
          </div>
        )}
      </DialogShell>

      {/* Targets list */}
      {targets.length > 0 ? (
        <div data-tour="targets-grid" className="space-y-4">
          {targets.map((target, i) => {
            const expectedCount = target.expectedSkillCount || sourceSkillCount;
            const isMergeOrCopy = target.mode === 'merge' && target.status === 'merged' || target.mode === 'copy' && target.status === 'copied';
            const hasDrift = isMergeOrCopy && target.linkedCount < expectedCount;
            const agentSummary = targetAgentSummary(target);
            const agentFilters = (target.agentInclude?.length ?? 0) + (target.agentExclude?.length ?? 0);
            const collectScope = getTargetCollectScope(target);
            const visibleAgentInclude = (target.agentInclude ?? []).slice(0, 3);
            const visibleAgentExclude = (target.agentExclude ?? []).slice(0, Math.max(0, 3 - visibleAgentInclude.length));
            const overflowAgentFilters = agentFilters - (visibleAgentInclude.length + visibleAgentExclude.length);
            return (
              <Card
                key={target.name}
                className={`!overflow-visible ${i % 2 === 0 ? 'rotate-[-0.15deg]' : 'rotate-[0.15deg]'}`}
                style={{ position: 'relative', zIndex: targets.length - i }}
              >
                {/* Top row: name + action icons */}
                <div className="flex items-center justify-between gap-4">
                  <div className="flex items-center gap-2 min-w-0">
                    <Target size={16} strokeWidth={2.5} className="text-success shrink-0" />
                    <span className="font-bold text-pencil">{target.name}</span>
                  </div>
                  <div className="flex items-center gap-1 shrink-0">
                    {collectScope && (
                      <IconButton
                        icon={<ArrowDownToLine size={16} strokeWidth={2.5} />}
                        label={`Collect local ${collectScope === 'both' ? 'resources' : collectScope === 'agent' ? 'agents' : 'skills'}`}
                        size="md"
                        variant="outline"
                        onClick={() => setCollecting({ name: target.name, scope: collectScope })}
                        className="hover:text-blue hover:border-blue"
                      />
                    )}
                    <IconButton
                      icon={<Trash2 size={16} strokeWidth={2.5} />}
                      label="Remove target"
                      size="md"
                      variant="danger-outline"
                      onClick={() => setRemoving(target.name)}
                    />
                  </div>
                </div>
                {/* ── Skills Section ── */}
                <div
                  className="mt-4 bg-muted/15 border border-pencil-light/15 px-4 py-3"
                  style={{ borderRadius: radius.md }}
                >
                  <div className="flex items-center gap-2 mb-2">
                    <span className="text-xs font-bold uppercase tracking-[0.12em] text-pencil shrink-0">
                      Skills
                    </span>
                    <StatusBadge status={target.status} />
                  </div>
                  <p className="font-mono text-sm text-pencil-light truncate mb-2">
                    {shortenHome(target.path)}
                  </p>
                  <div className="flex items-center gap-2">
                    <Select
                      value={target.mode || 'merge'}
                      onChange={(mode) => updateTargetSetting(target.name, { mode }, `Sync mode changed to ${mode}`)}
                      options={SYNC_MODE_OPTIONS}
                      size="sm"
                      className="w-44"
                    />
                    {target.mode !== 'symlink' && (
                      <Select
                        value={target.targetNaming || 'flat'}
                        onChange={(naming) => updateTargetSetting(target.name, { target_naming: naming }, `Target naming changed to ${naming}`)}
                        options={TARGET_NAMING_OPTIONS}
                        size="sm"
                        className="w-48"
                      />
                    )}
                    {(target.mode === 'merge' || target.mode === 'copy') && (
                      <span className={`text-sm ml-auto shrink-0 ${hasDrift ? 'text-warning' : 'text-muted-dark'}`}>
                        {hasDrift ? (
                          <span className="flex items-center gap-1">
                            <AlertTriangle size={12} strokeWidth={2.5} />
                            {target.linkedCount}/{expectedCount} {target.mode === 'copy' ? 'managed' : 'shared'}, {target.localCount} local
                          </span>
                        ) : (
                          <>{target.linkedCount} {target.mode === 'copy' ? 'managed' : 'shared'}, {target.localCount} local</>
                        )}
                      </span>
                    )}
                  </div>
                  {(target.mode === 'merge' || target.mode === 'copy') && (
                    <div {...(i === 0 ? { 'data-tour': 'skill-filters' } : {})} className="mt-2 flex items-center gap-2 flex-wrap">
                      <span className="text-sm text-pencil-light">
                        {(() => {
                          const summary = getTargetSummary(target.name);
                          const hasFilters = target.include?.length || target.exclude?.length;
                          if (summary.total === 0) return 'No skills';
                          if (!hasFilters) return `All ${summary.total} skills`;
                          return `${summary.synced}/${summary.total} skills`;
                        })()}
                      </span>
                      {(() => {
                        const inc = target.include ?? [];
                        const exc = target.exclude ?? [];
                        const MAX_TAGS = 3;
                        const visibleInc = inc.slice(0, MAX_TAGS);
                        const visibleExc = exc.slice(0, Math.max(0, MAX_TAGS - visibleInc.length));
                        const overflow = (inc.length + exc.length) - (visibleInc.length + visibleExc.length);
                        return (
                          <>
                            {visibleInc.map((p, pi) => (
                              <span key={`inc-${pi}`} className="text-xs font-bold text-blue bg-info-light px-2 py-0.5 border border-blue/30" style={{ borderRadius: radius.sm }}>
                                + {p}
                              </span>
                            ))}
                            {visibleExc.map((p, pi) => (
                              <span key={`exc-${pi}`} className="text-xs font-bold text-danger bg-danger-light px-2 py-0.5 border border-danger/30" style={{ borderRadius: radius.sm }}>
                                − {p}
                              </span>
                            ))}
                            {overflow > 0 && (
                              <span className="text-xs text-pencil-light">+{overflow} more</span>
                            )}
                          </>
                        );
                      })()}
                      <Link
                        to={`/targets/${encodeURIComponent(target.name)}/filters?kind=skill`}
                        className="text-xs font-bold text-blue hover:underline"
                      >
                        {(target.include?.length || target.exclude?.length) ? 'Edit in Filter Studio →' : 'Customize filters →'}
                      </Link>
                    </div>
                  )}
                </div>

                {/* ── Agents Section ── */}
                {target.agentPath && (
                  <div
                    className="mt-3 bg-muted/15 border border-pencil-light/15 px-4 py-3"
                    style={{ borderRadius: radius.md }}
                  >
                    <div className="flex items-center gap-2 mb-2">
                      <span className="text-xs font-bold uppercase tracking-[0.12em] text-pencil shrink-0">
                        Agents
                      </span>
                      {(() => {
                        const agentExpected = target.agentExpectedCount ?? 0;
                        const agentLinked = target.agentLinkedCount ?? 0;
                        const mode = target.agentMode || 'merge';
                        if (agentLinked > 0 || agentExpected > 0) {
                          const label = mode === 'merge' ? 'merged' : mode === 'copy' ? 'copied' : 'linked';
                          return <StatusBadge status={label} />;
                        }
                        return null;
                      })()}
                    </div>
                    <p className="font-mono text-sm text-pencil-light truncate mb-2">
                      {shortenHome(target.agentPath)}
                    </p>
                    <div className="flex items-center gap-2">
                      <Select
                        value={target.agentMode || 'merge'}
                        onChange={(mode) => updateTargetSetting(target.name, { agent_mode: mode }, `Agent mode changed to ${mode}`)}
                        options={AGENT_MODE_OPTIONS}
                        size="sm"
                        className="w-44"
                      />
                      <span className={`text-sm ml-auto shrink-0 ${agentSummary.hasDrift ? 'text-warning' : 'text-muted-dark'}`}>
                        {agentSummary.hasDrift ? (
                          <span className="flex items-center gap-1">
                            <AlertTriangle size={12} strokeWidth={2.5} />
                            {agentSummary.text}
                          </span>
                        ) : (
                          agentSummary.text
                        )}
                      </span>
                    </div>
                    <div className="mt-2 flex items-center gap-2 flex-wrap">
                      <span className="text-sm text-pencil-light">{targetAgentAvailabilityText(target)}</span>
                      {visibleAgentInclude.map((pattern, idx) => (
                        <span
                          key={`agent-inc-${idx}`}
                          className="text-xs font-bold text-blue bg-info-light px-2 py-0.5 border border-blue/30"
                          style={{ borderRadius: radius.sm }}
                        >
                          + {pattern}
                        </span>
                      ))}
                      {visibleAgentExclude.map((pattern, idx) => (
                        <span
                          key={`agent-exc-${idx}`}
                          className="text-xs font-bold text-danger bg-danger-light px-2 py-0.5 border border-danger/30"
                          style={{ borderRadius: radius.sm }}
                        >
                          − {pattern}
                        </span>
                      ))}
                      {overflowAgentFilters > 0 && (
                        <span className="text-xs text-pencil-light">+{overflowAgentFilters} more</span>
                      )}
                      <Link
                        to={`/targets/${encodeURIComponent(target.name)}/filters?kind=agent`}
                        className="text-xs font-bold text-blue hover:underline"
                      >
                        {agentFilters ? 'Edit in Filter Studio →' : 'Customize filters →'}
                      </Link>
                    </div>
                  </div>
                )}
                {(target.skippedSkillCount ?? 0) > 0 && (
                  <p className="mt-1 text-xs text-warning flex items-center gap-1">
                    <AlertTriangle size={11} strokeWidth={2.5} />
                    {target.skippedSkillCount} skill(s) skipped
                    {(target.collisionCount ?? 0) > 0 && <> ({target.collisionCount} name collision(s))</>}
                    {' '}— try switching to <strong>Flat</strong> naming to resolve
                  </p>
                )}
              </Card>
            );
          })}
        </div>
      ) : (
        <EmptyState
          icon={Target}
          title="No targets configured"
          description="Add a target to start syncing your skills."
          action={
            !adding ? (
              <Button onClick={() => setAdding(true)} variant="secondary" size="sm">
                <Plus size={16} strokeWidth={2.5} />
                Add Your First Target
              </Button>
            ) : undefined
          }
        />
      )}

      {/* Confirm remove dialog */}
      <ConfirmDialog
        open={!!removing}
        title="Remove Target"
        message={`Remove target "${removing}"? Skills will no longer sync to it.`}
        confirmText="Remove"
        variant="danger"
        onConfirm={() => removing && handleRemove(removing)}
        onCancel={() => setRemoving(null)}
      />

      {/* Confirm collect dialog */}
      <ConfirmDialog
        open={!!collecting}
        title={`Collect Local ${collecting?.scope === 'both' ? 'Resources' : collecting?.scope === 'agent' ? 'Agents' : 'Skills'}`}
        message={collecting
          ? `Scan "${collecting.name}" for local ${collecting.scope === 'both' ? 'resources' : collecting.scope === 'agent' ? 'agents' : 'skills'} to collect back to source?`
          : ''}
        confirmText="Scan"
        onConfirm={() => {
          if (collecting) {
            const params = new URLSearchParams({
              target: collecting.name,
              scope: collecting.scope,
            });
            navigate(`/collect?${params.toString()}`);
          }
          setCollecting(null);
        }}
        onCancel={() => setCollecting(null)}
      />
    </div>
  );
}

/** Clickable row inside the target picker list */
function TargetPickerItem({
  target,
  isDetected,
  onSelect,
}: {
  target: AvailableTarget;
  isDetected?: boolean;
  onSelect: (target: AvailableTarget) => void;
}) {
  return (
    <button
      onClick={() => onSelect(target)}
      className="w-full text-left px-3 py-2.5 flex items-center gap-3 border-b border-muted/60 hover:bg-muted/20 transition-colors cursor-pointer group"
    >
      {isDetected ? (
        <span className="w-2.5 h-2.5 rounded-full bg-success shrink-0" />
      ) : (
        <span className="w-2.5 h-2.5 rounded-full border-2 border-muted-dark shrink-0" />
      )}
      <div className="min-w-0 flex-1">
        <span className="font-bold text-pencil group-hover:text-blue transition-colors">
          {target.name}
        </span>
        {target.agentPath ? (
          <div className="mt-0.5 space-y-0.5">
            <p className="font-mono text-xs text-pencil-light truncate">
              <span className="text-[10px] font-bold uppercase tracking-wider text-pencil-light/70 mr-1.5">Skills</span>
              {shortenHome(target.path)}
            </p>
            <p className="font-mono text-xs text-pencil-light truncate">
              <span className="text-[10px] font-bold uppercase tracking-wider text-pencil-light/70 mr-1.5">Agents</span>
              {shortenHome(target.agentPath)}
            </p>
          </div>
        ) : (
          <p className="font-mono text-xs text-pencil-light truncate mt-0.5">
            {shortenHome(target.path)}
          </p>
        )}
      </div>
      {isDetected && (
        <span
          className="text-xs text-success bg-success-light px-2 py-0.5 shrink-0"
          style={{ borderRadius: radius.sm }}
        >
          detected
        </span>
      )}
    </button>
  );
}
