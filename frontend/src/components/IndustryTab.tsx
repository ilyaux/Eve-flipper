import { useState, useCallback, useRef, useMemo, useEffect, lazy, Suspense } from "react";
import { useI18n } from "@/lib/i18n";
import {
  analyzeIndustry,
  searchBuildableItems,
  getStations,
  getStructures,
  getAuthIndustryProjects,
  createAuthIndustryProject,
  getAuthIndustryProjectSnapshot,
  previewAuthIndustryProjectPlan,
  planAuthIndustryProject,
  rebalanceAuthIndustryProjectMaterials,
  syncAuthIndustryProjectBlueprintPool,
  getAuthIndustryCoverage,
  getAuthIndustryLedger,
  updateAuthIndustryTaskStatus,
  updateAuthIndustryTaskStatusBulk,
  updateAuthIndustryTaskPriority,
  updateAuthIndustryTaskPriorityBulk,
  updateAuthIndustryJobStatus,
  updateAuthIndustryJobStatusBulk,
} from "@/lib/api";
import type {
  IndustryAnalysis,
  IndustryCoverageBlueprintNeed,
  IndustryCoverageMaterialNeed,
  IndustryCoverageResult,
  IndustryActivityStep,
  IndustryParams,
  FlatMaterial,
  BuildableItem,
  StationInfo,
  IndustryProject,
  IndustryProjectSnapshot,
  IndustryLedger,
  IndustryTaskStatus,
  IndustryJobStatus,
  IndustryPlanPatch,
  IndustryPlanPreview,
  IndustryPlanSchedulerInput,
  IndustryTaskPlanInput,
  IndustryJobPlanInput,
  IndustryMaterialPlanInput,
  IndustryBlueprintPoolInput,
} from "@/lib/types";
import { formatISK } from "@/lib/format";
import {
  TabSettingsPanel,
  SettingsField,
  SettingsNumberInput,
  SettingsGrid,
  SettingsCheckbox,
  SettingsSelect,
} from "./TabSettingsPanel";
import { SystemAutocomplete } from "./SystemAutocomplete";
import { EmptyState } from "./EmptyState";
import { useGlobalToast } from "./Toast";
import { ExecutionPlannerPopup } from "./ExecutionPlannerPopup";
import {
  formatUtcShort,
  industryJobStatusClass,
  industryTaskStatusClass,
  planPatchSignature,
  taskConstraintRecord,
  taskConstraintNumber,
  type IndustryPlannerWarningEvent,
  type IndustryPlannerWarningSource,
  type IndustryTaskDependencyBoard,
} from "./industry/industryHelpers";
import type { IndustryJobsWorkspaceTab } from "./industry/IndustryJobsWorkspaceNav";

const IndustryJobsLedgerPanel = lazy(async () => {
  const mod = await import("./industry/IndustryJobsLedgerPanel");
  return { default: mod.IndustryJobsLedgerPanel };
});

const IndustryAnalysisResultsPanel = lazy(async () => {
  const mod = await import("./industry/IndustryAnalysisResultsPanel");
  return { default: mod.IndustryAnalysisResultsPanel };
});

// Highlight matching text in search results
function HighlightMatch({ text, query }: { text: string; query: string }) {
  if (!query.trim()) return <>{text}</>;
  
  const lowerText = text.toLowerCase();
  const lowerQuery = query.toLowerCase().trim();
  const index = lowerText.indexOf(lowerQuery);
  
  if (index === -1) return <>{text}</>;
  
  return (
    <>
      {text.slice(0, index)}
      <span className="text-eve-accent font-medium">{text.slice(index, index + query.length)}</span>
      {text.slice(index + query.length)}
    </>
  );
}

interface Props {
  onError?: (msg: string) => void;
  isLoggedIn?: boolean;
}

type IndustryInnerTab = "analysis" | "jobs";
type PlanBuilderSection = "tasks" | "jobs" | "materials" | "blueprints";
type IndustryStrategyPreset = "conservative" | "balanced" | "aggressive";
type IndustryActivityMode = "auto" | "manufacturing" | "reaction" | "invention";

const INDUSTRY_LEDGER_SELECTED_PROJECT_STORAGE_KEY = "eve-flipper-industry-selected-project-id";
const INDUSTRY_SCHEDULER_DEFAULTS: Record<
  IndustryStrategyPreset,
  {
    slotCount: number;
    maxRunsPerJob: number;
    maxDurationHours: number;
    queueStatus: IndustryJobStatus;
  }
> = {
  conservative: {
    slotCount: 1,
    maxRunsPerJob: 50,
    maxDurationHours: 12,
    queueStatus: "planned",
  },
  balanced: {
    slotCount: 2,
    maxRunsPerJob: 200,
    maxDurationHours: 24,
    queueStatus: "queued",
  },
  aggressive: {
    slotCount: 4,
    maxRunsPerJob: 400,
    maxDurationHours: 72,
    queueStatus: "queued",
  },
};

function readStoredIndustryLedgerProjectID(): number {
  try {
    const raw = localStorage.getItem(INDUSTRY_LEDGER_SELECTED_PROJECT_STORAGE_KEY);
    const parsed = Number(raw);
    if (Number.isFinite(parsed) && parsed > 0) {
      return Math.round(parsed);
    }
  } catch {
    // ignore storage access errors
  }
  return 0;
}

function persistIndustryLedgerProjectID(projectID: number): void {
  try {
    if (projectID > 0) {
      localStorage.setItem(INDUSTRY_LEDGER_SELECTED_PROJECT_STORAGE_KEY, String(projectID));
      return;
    }
    localStorage.removeItem(INDUSTRY_LEDGER_SELECTED_PROJECT_STORAGE_KEY);
  } catch {
    // ignore storage access errors
  }
}

function schedulerDefaultsForStrategy(strategy?: string) {
  const normalized = (strategy ?? "").toLowerCase() as IndustryStrategyPreset;
  return INDUSTRY_SCHEDULER_DEFAULTS[normalized] ?? INDUSTRY_SCHEDULER_DEFAULTS.balanced;
}

function buildIndustryCoverageNeeds(analysis: IndustryAnalysis): {
  materials: IndustryCoverageMaterialNeed[];
  blueprints: IndustryCoverageBlueprintNeed[];
} {
  const materials = (analysis.flat_materials ?? [])
    .filter((m) => m.type_id > 0 && m.quantity > 0)
    .map((m) => ({
      type_id: m.type_id,
      type_name: m.type_name,
      required_qty: Math.ceil(m.quantity),
    }));

  const bpByID = new Map<number, IndustryCoverageBlueprintNeed>();
  for (const step of analysis.activity_plan ?? []) {
    if (!step.blueprint_type_id || step.blueprint_type_id <= 0) continue;
    const requiredRuns = Math.max(
      1,
      Math.ceil(step.activity === "invention" && step.expected_attempts ? step.expected_attempts : step.runs || 1)
    );
    const existing = bpByID.get(step.blueprint_type_id);
    bpByID.set(step.blueprint_type_id, {
      blueprint_type_id: step.blueprint_type_id,
      blueprint_name: step.blueprint_name || existing?.blueprint_name || "",
      activity: existing?.activity && existing.activity !== step.activity ? "mixed" : step.activity || existing?.activity || "",
      required_runs: (existing?.required_runs ?? 0) + requiredRuns,
    });
  }

  const rootBlueprintID = analysis.material_tree?.blueprint?.blueprint_type_id ?? 0;
  if (bpByID.size === 0 && rootBlueprintID > 0) {
    bpByID.set(rootBlueprintID, {
      blueprint_type_id: rootBlueprintID,
      blueprint_name: "",
      activity: analysis.material_tree?.blueprint?.activity || analysis.activity_mode || "manufacturing",
      required_runs: Math.max(1, Math.ceil(analysis.runs || 1)),
    });
  }

  return {
    materials,
    blueprints: Array.from(bpByID.values()),
  };
}

function industryStepRuns(step: IndustryActivityStep): number {
  if (step.activity === "invention" && step.expected_attempts) {
    return Math.max(1, Math.ceil(step.expected_attempts));
  }
  return Math.max(1, Math.ceil(step.runs || 1));
}

function industryStepLabel(step: IndustryActivityStep): string {
  const activity = step.activity || "industry";
  const product = step.product_name || `Type ${step.product_type_id}`;
  return `${activity} ${product}`;
}

export function IndustryTab({ onError, isLoggedIn = false }: Props) {
  const { t } = useI18n();
  const { addToast } = useGlobalToast();

  // Search state
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<BuildableItem[]>([]);
  const [searching, setSearching] = useState(false);
  const [showDropdown, setShowDropdown] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState(-1);
  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const searchAbortRef = useRef<AbortController | null>(null);
  const searchRequestSeqRef = useRef(0);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Selected item
  const [selectedItem, setSelectedItem] = useState<BuildableItem | null>(null);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node) &&
          inputRef.current && !inputRef.current.contains(e.target as Node)) {
        setShowDropdown(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // Parameters
  const [runs, setRuns] = useState(1);
  const [activityMode, setActivityMode] = useState<IndustryActivityMode>("auto");
  const [me, setME] = useState(10);
  const [te, setTE] = useState(20);
  const [systemName, setSystemName] = useState("Jita");
  const [facilityTax, setFacilityTax] = useState(0);
  const [structureBonus, setStructureBonus] = useState(1);
  const [brokerFee, setBrokerFee] = useState(3);
  const [salesTaxPercent, setSalesTaxPercent] = useState(8);
  const [ownBlueprint, setOwnBlueprint] = useState(true);
  const [blueprintCost, setBlueprintCost] = useState(0);
  const [blueprintIsBPO, setBlueprintIsBPO] = useState(true);
  const [inventionChance, setInventionChance] = useState(0);
  const [decryptorCost, setDecryptorCost] = useState(0);
  const [inventionOutputRuns, setInventionOutputRuns] = useState(0);

  // Station/Structure selection
  const [stations, setStations] = useState<StationInfo[]>([]);
  const [selectedStationId, setSelectedStationId] = useState<number>(0);
  const [loadingStations, setLoadingStations] = useState(false);
  const [systemRegionId, setSystemRegionId] = useState<number>(0);
  const [systemId, setSystemId] = useState<number>(0);
  const [includeStructures, setIncludeStructures] = useState(false);
  const [structureStations, setStructureStations] = useState<StationInfo[]>([]);
  const [loadingStructures, setLoadingStructures] = useState(false);
  const stationsAbortRef = useRef<AbortController | null>(null);
  const stationsRequestSeqRef = useRef(0);
  const structuresAbortRef = useRef<AbortController | null>(null);
  const structuresRequestSeqRef = useRef(0);
  const selectedStationLabel = useMemo(() => {
    if (selectedStationId <= 0) return "";
    const station = [...stations, ...structureStations].find((s) => s.id === selectedStationId);
    return station?.name || `Location ${selectedStationId}`;
  }, [selectedStationId, stations, structureStations]);

  // Analysis state
  const [analyzing, setAnalyzing] = useState(false);
  const [progress, setProgress] = useState("");
  const [result, setResult] = useState<IndustryAnalysis | null>(null);
  const [industryCoverage, setIndustryCoverage] = useState<IndustryCoverageResult | null>(null);
  const [industryCoverageLoading, setIndustryCoverageLoading] = useState(false);
  const [industryCoverageMeta, setIndustryCoverageMeta] = useState("");
  const abortRef = useRef<AbortController | null>(null);

  // View mode
  const [viewMode, setViewMode] = useState<"tree" | "shopping">("tree");

  // Execution plan popup (from shopping list)
  const [execPlanMaterial, setExecPlanMaterial] = useState<FlatMaterial | null>(null);

  // Industry ledger (M1) state
  const [ledgerProjects, setLedgerProjects] = useState<IndustryProject[]>([]);
  const [ledgerProjectsLoading, setLedgerProjectsLoading] = useState(false);
  const [ledgerLoading, setLedgerLoading] = useState(false);
  const [selectedLedgerProjectId, setSelectedLedgerProjectId] = useState(() => readStoredIndustryLedgerProjectID());
  const [ledgerData, setLedgerData] = useState<IndustryLedger | null>(null);
  const [ledgerSnapshot, setLedgerSnapshot] = useState<IndustryProjectSnapshot | null>(null);
  const [ledgerSnapshotLoading, setLedgerSnapshotLoading] = useState(false);
  const [newLedgerProjectName, setNewLedgerProjectName] = useState("");
  const [newLedgerProjectStrategy, setNewLedgerProjectStrategy] = useState<"conservative" | "balanced" | "aggressive">("balanced");
  const [creatingLedgerProject, setCreatingLedgerProject] = useState(false);
  const [updatingLedgerTaskId, setUpdatingLedgerTaskId] = useState(0);
  const [updatingLedgerTasksBulk, setUpdatingLedgerTasksBulk] = useState(false);
  const [selectedLedgerTaskIDs, setSelectedLedgerTaskIDs] = useState<number[]>([]);
  const [bulkLedgerTaskPriority, setBulkLedgerTaskPriority] = useState(100);
  const [updatingLedgerJobId, setUpdatingLedgerJobId] = useState(0);
  const [updatingLedgerJobsBulk, setUpdatingLedgerJobsBulk] = useState(false);
  const [selectedLedgerJobIDs, setSelectedLedgerJobIDs] = useState<number[]>([]);
  const [rebalancingLedgerMaterials, setRebalancingLedgerMaterials] = useState(false);
  const [syncingLedgerBlueprintPool, setSyncingLedgerBlueprintPool] = useState(false);
  const [rebalanceInventoryScope, setRebalanceInventoryScope] = useState<"single" | "all">("single");
  const [rebalanceLookbackDays, setRebalanceLookbackDays] = useState(180);
  const [rebalanceStrategy, setRebalanceStrategy] = useState<"preserve" | "buy" | "build">("preserve");
  const [rebalanceWarehouseScope, setRebalanceWarehouseScope] = useState<"global" | "location_first" | "strict_location">("location_first");
  const [blueprintSyncDefaultBPCRuns, setBlueprintSyncDefaultBPCRuns] = useState(1);
  const [rebalanceUseSelectedStation, setRebalanceUseSelectedStation] = useState(false);
  const [applyingLedgerPlan, setApplyingLedgerPlan] = useState(false);
  const [replaceLedgerPlanOnApply, setReplaceLedgerPlanOnApply] = useState(true);
  const [lastLedgerPlanSummary, setLastLedgerPlanSummary] = useState("");
  const [previewingLedgerPlan, setPreviewingLedgerPlan] = useState(false);
  const [lastLedgerPlanPreview, setLastLedgerPlanPreview] = useState<IndustryPlanPreview | null>(null);
  const [lastLedgerPlanPreviewPatch, setLastLedgerPlanPreviewPatch] = useState<IndustryPlanPatch | null>(null);
  const [useVisualPlanBuilder, setUseVisualPlanBuilder] = useState(true);
  const [planDraftTasks, setPlanDraftTasks] = useState<IndustryTaskPlanInput[]>([]);
  const [planDraftJobs, setPlanDraftJobs] = useState<IndustryJobPlanInput[]>([]);
  const [planDraftMaterials, setPlanDraftMaterials] = useState<IndustryMaterialPlanInput[]>([]);
  const [planDraftBlueprints, setPlanDraftBlueprints] = useState<IndustryBlueprintPoolInput[]>([]);
  const [strictBlueprintBindingMode, setStrictBlueprintBindingMode] = useState(true);
  const [plannerWarnings, setPlannerWarnings] = useState<IndustryPlannerWarningEvent[]>([]);
  const plannerWarningSeqRef = useRef(1);
  const ledgerLoadSeqRef = useRef(0);
  const schedulerDefaultsProjectKeyRef = useRef("");
  const [industryInnerTab, setIndustryInnerTab] = useState<IndustryInnerTab>("analysis");
  const [jobsWorkspaceTab, setJobsWorkspaceTab] = useState<IndustryJobsWorkspaceTab>("guide");
  const [planBuilderCompactMode, setPlanBuilderCompactMode] = useState(true);
  const [planBuilderPageSize, setPlanBuilderPageSize] = useState(6);
  const [enablePlanScheduler, setEnablePlanScheduler] = useState(true);
  const [schedulerSlotCount, setSchedulerSlotCount] = useState(2);
  const [schedulerMaxRunsPerJob, setSchedulerMaxRunsPerJob] = useState(200);
  const [schedulerMaxDurationHours, setSchedulerMaxDurationHours] = useState(24);
  const [schedulerQueueStatus, setSchedulerQueueStatus] = useState<IndustryJobStatus>("queued");
  const [planBuilderCollapsed, setPlanBuilderCollapsed] = useState<Record<PlanBuilderSection, boolean>>({
    tasks: false,
    jobs: false,
    materials: false,
    blueprints: false,
  });
  const [planBuilderPage, setPlanBuilderPage] = useState<Record<PlanBuilderSection, number>>({
    tasks: 1,
    jobs: 1,
    materials: 1,
    blueprints: 1,
  });

  useEffect(() => () => {
    clearTimeout(searchTimeoutRef.current);
    searchAbortRef.current?.abort();
    stationsAbortRef.current?.abort();
    structuresAbortRef.current?.abort();
    abortRef.current?.abort();
  }, []);

  // Load stations when system changes
  useEffect(() => {
    stationsAbortRef.current?.abort();
    stationsRequestSeqRef.current += 1;
    const reqSeq = stationsRequestSeqRef.current;
    const normalizedSystem = systemName.trim();
    if (!normalizedSystem) {
      setStations([]);
      setSystemRegionId(0);
      setSystemId(0);
      setSelectedStationId(0);
      setStructureStations([]);
      setLoadingStations(false);
      return;
    }
    const controller = new AbortController();
    stationsAbortRef.current = controller;
    setLoadingStations(true);
    setStructureStations([]);
    getStations(normalizedSystem, controller.signal)
      .then((resp) => {
        if (reqSeq !== stationsRequestSeqRef.current) return;
        setStations(resp.stations);
        setSystemRegionId(resp.region_id);
        setSystemId(resp.system_id);
        setSelectedStationId(0);
        setStructureStations([]);
      })
      .catch((e: unknown) => {
        if (reqSeq !== stationsRequestSeqRef.current) return;
        if (e instanceof Error && e.name === "AbortError") return;
        setStations([]);
        setSystemRegionId(0);
        setSystemId(0);
      })
      .finally(() => {
        if (reqSeq === stationsRequestSeqRef.current) {
          setLoadingStations(false);
        }
      });
  }, [systemName]);

  // Fetch structures when toggle is enabled
  useEffect(() => {
    structuresAbortRef.current?.abort();
    structuresRequestSeqRef.current += 1;
    const reqSeq = structuresRequestSeqRef.current;
    if (!includeStructures || !systemId || !systemRegionId) {
      setStructureStations([]);
      setLoadingStructures(false);
      return;
    }
    const controller = new AbortController();
    structuresAbortRef.current = controller;
    setLoadingStructures(true);
    getStructures(systemId, systemRegionId, controller.signal)
      .then((rows) => {
        if (reqSeq !== structuresRequestSeqRef.current) return;
        setStructureStations(rows);
      })
      .catch((e: unknown) => {
        if (reqSeq !== structuresRequestSeqRef.current) return;
        if (e instanceof Error && e.name === "AbortError") return;
        setStructureStations([]);
      })
      .finally(() => {
        if (reqSeq === structuresRequestSeqRef.current) {
          setLoadingStructures(false);
        }
      });
  }, [includeStructures, systemId, systemRegionId]);

  // Combined stations (NPC + structures when toggle is on)
  const allStations = useMemo(() => {
    if (includeStructures && structureStations.length > 0) {
      return [...stations, ...structureStations];
    }
    return stations;
  }, [stations, structureStations, includeStructures]);

  useEffect(() => {
    if (selectedStationId <= 0) return;
    const exists = allStations.some((station) => Number(station.id) === selectedStationId);
    if (!exists) {
      setSelectedStationId(0);
    }
  }, [allStations, selectedStationId]);

  const refreshLedgerProjects = useCallback(async (preferredProjectId?: number) => {
    if (!isLoggedIn) {
      persistIndustryLedgerProjectID(0);
      setLedgerProjects([]);
      setSelectedLedgerProjectId(0);
      setLedgerData(null);
      setLedgerSnapshot(null);
      setSelectedLedgerTaskIDs([]);
      setSelectedLedgerJobIDs([]);
      return;
    }

    setLedgerProjectsLoading(true);
    try {
      const resp = await getAuthIndustryProjects({ limit: 120 });
      const projects = Array.isArray(resp.projects) ? resp.projects : [];
      const storedProjectID = readStoredIndustryLedgerProjectID();
      setLedgerProjects(projects);
      setSelectedLedgerProjectId((current) => {
        const candidates = [preferredProjectId ?? 0, current, storedProjectID];
        for (const candidate of candidates) {
          if (candidate > 0 && projects.some((p) => p.id === candidate)) {
            return candidate;
          }
        }
        return Number(projects[0]?.id ?? 0);
      });
    } catch (e: unknown) {
      console.error("Industry projects load error:", e);
      onError?.(e instanceof Error ? e.message : "Failed to load industry projects");
      persistIndustryLedgerProjectID(0);
      setLedgerProjects([]);
      setSelectedLedgerProjectId(0);
      setLedgerData(null);
      setLedgerSnapshot(null);
      setSelectedLedgerTaskIDs([]);
      setSelectedLedgerJobIDs([]);
    } finally {
      setLedgerProjectsLoading(false);
    }
  }, [isLoggedIn, onError]);

  const refreshLedger = useCallback(async (projectId: number) => {
    if (!isLoggedIn || projectId <= 0) {
      ledgerLoadSeqRef.current += 1;
      setLedgerData(null);
      setLedgerSnapshot(null);
      setSelectedLedgerTaskIDs([]);
      setSelectedLedgerJobIDs([]);
      return;
    }

    const loadSeq = ledgerLoadSeqRef.current + 1;
    ledgerLoadSeqRef.current = loadSeq;

    setLedgerLoading(true);
    setLedgerSnapshotLoading(true);
    setLedgerData(null);
    setLedgerSnapshot(null);
    setSelectedLedgerTaskIDs([]);
    setSelectedLedgerJobIDs([]);

    try {
      const [ledgerResult, snapshotResult] = await Promise.allSettled([
        getAuthIndustryLedger({ project_id: projectId, limit: 200 }),
        getAuthIndustryProjectSnapshot(projectId),
      ]);
      if (loadSeq !== ledgerLoadSeqRef.current) {
        return;
      }

      if (ledgerResult.status !== "fulfilled") {
        throw ledgerResult.reason;
      }
      setLedgerData(ledgerResult.value);
      setSelectedLedgerTaskIDs([]);
      setSelectedLedgerJobIDs([]);

      if (snapshotResult.status === "fulfilled") {
        setLedgerSnapshot(snapshotResult.value);
      } else {
        console.error("Industry snapshot load error:", snapshotResult.reason);
        setLedgerSnapshot(null);
      }
    } catch (e: unknown) {
      if (loadSeq !== ledgerLoadSeqRef.current) {
        return;
      }
      console.error("Industry ledger load error:", e);
      onError?.(e instanceof Error ? e.message : "Failed to load industry ledger");
      setLedgerData(null);
      setLedgerSnapshot(null);
      setSelectedLedgerTaskIDs([]);
      setSelectedLedgerJobIDs([]);
    } finally {
      if (loadSeq === ledgerLoadSeqRef.current) {
        setLedgerLoading(false);
        setLedgerSnapshotLoading(false);
      }
    }
  }, [isLoggedIn, onError]);

  useEffect(() => {
    refreshLedgerProjects();
  }, [refreshLedgerProjects]);

  useEffect(() => {
    if (!isLoggedIn || selectedLedgerProjectId <= 0) {
      ledgerLoadSeqRef.current += 1;
      setLedgerData(null);
      setLedgerSnapshot(null);
      setSelectedLedgerTaskIDs([]);
      setSelectedLedgerJobIDs([]);
      return;
    }
    refreshLedger(selectedLedgerProjectId);
  }, [isLoggedIn, selectedLedgerProjectId, refreshLedger]);

  const pushPlannerWarnings = useCallback((source: IndustryPlannerWarningSource, warnings: string[] | string) => {
    const list = Array.isArray(warnings) ? warnings : [warnings];
    const normalized = list
      .map((entry) => String(entry ?? "").trim())
      .filter((entry) => entry.length > 0);
    if (normalized.length === 0) {
      return;
    }
    const nowISO = new Date().toISOString();
    setPlannerWarnings((prev) => {
      const next = [...prev];
      for (const message of normalized) {
        const duplicateIndex = next.findIndex((item) => item.source === source && item.message === message);
        if (duplicateIndex >= 0) {
          next[duplicateIndex] = {
            ...next[duplicateIndex],
            created_at: nowISO,
          };
          continue;
        }
        next.unshift({
          id: plannerWarningSeqRef.current++,
          source,
          message,
          created_at: nowISO,
        });
      }
      return next.slice(0, 40);
    });
  }, []);

  useEffect(() => {
    if (!isLoggedIn) {
      persistIndustryLedgerProjectID(0);
      return;
    }
    persistIndustryLedgerProjectID(selectedLedgerProjectId);
  }, [isLoggedIn, selectedLedgerProjectId]);

  useEffect(() => {
    setSelectedLedgerTaskIDs([]);
    setSelectedLedgerJobIDs([]);
    setPlannerWarnings([]);
    setLastLedgerPlanSummary("");
    setLastLedgerPlanPreview(null);
    setLastLedgerPlanPreviewPatch(null);
    setPlanDraftTasks([]);
    setPlanDraftJobs([]);
    setPlanDraftMaterials([]);
    setPlanDraftBlueprints([]);
    setPlanBuilderPage({
      tasks: 1,
      jobs: 1,
      materials: 1,
      blueprints: 1,
    });
    setJobsWorkspaceTab("guide");
  }, [selectedLedgerProjectId]);

  useEffect(() => {
    if (selectedLedgerProjectId <= 0) {
      schedulerDefaultsProjectKeyRef.current = "";
      return;
    }
    const selectedProject = ledgerProjects.find((project) => project.id === selectedLedgerProjectId);
    const strategy = selectedProject?.strategy ?? "balanced";
    const projectKey = `${selectedLedgerProjectId}:${strategy}`;
    if (schedulerDefaultsProjectKeyRef.current === projectKey) {
      return;
    }
    schedulerDefaultsProjectKeyRef.current = projectKey;
    const defaults = schedulerDefaultsForStrategy(strategy);
    setSchedulerSlotCount(defaults.slotCount);
    setSchedulerMaxRunsPerJob(defaults.maxRunsPerJob);
    setSchedulerMaxDurationHours(defaults.maxDurationHours);
    setSchedulerQueueStatus(defaults.queueStatus);
  }, [selectedLedgerProjectId, ledgerProjects]);

  const handleCreateLedgerProject = useCallback(async () => {
    if (!isLoggedIn) return;
    const name = newLedgerProjectName.trim();
    if (!name) {
      addToast(t("industryLedgerEnterProjectName"), "warning", 2000);
      return;
    }
    setCreatingLedgerProject(true);
    try {
      const created = await createAuthIndustryProject({
        name,
        strategy: newLedgerProjectStrategy,
      });
      const createdProjectID = Number(created.project?.id ?? 0);
      setNewLedgerProjectName("");
      addToast(t("industryLedgerProjectCreated"), "success", 1800);
      if (createdProjectID > 0) {
        setSelectedLedgerProjectId(createdProjectID);
      }
      await refreshLedgerProjects(createdProjectID > 0 ? createdProjectID : undefined);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to create project";
      onError?.(msg);
      addToast(msg, "error", 2400);
    } finally {
      setCreatingLedgerProject(false);
    }
  }, [isLoggedIn, newLedgerProjectName, newLedgerProjectStrategy, addToast, onError, refreshLedgerProjects, t]);

  const handleSetLedgerTaskStatus = useCallback(async (taskId: number, status: IndustryTaskStatus) => {
    if (!isLoggedIn || selectedLedgerProjectId <= 0 || taskId <= 0) return;
    setUpdatingLedgerTaskId(taskId);
    try {
      await updateAuthIndustryTaskStatus({ task_id: taskId, status });
      addToast(`Task #${taskId} -> ${status}`, "success", 1500);
      await refreshLedger(selectedLedgerProjectId);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to update task status";
      onError?.(msg);
      addToast(msg, "error", 2500);
    } finally {
      setUpdatingLedgerTaskId(0);
    }
  }, [isLoggedIn, selectedLedgerProjectId, refreshLedger, addToast, onError]);

  const handleSetLedgerTaskPriority = useCallback(async (taskId: number, priority: number) => {
    if (!isLoggedIn || selectedLedgerProjectId <= 0 || taskId <= 0) return;
    const normalizedPriority = Number.isFinite(priority) ? Math.round(priority) : 0;
    setUpdatingLedgerTaskId(taskId);
    try {
      await updateAuthIndustryTaskPriority({ task_id: taskId, priority: normalizedPriority });
      addToast(`Task #${taskId} priority -> ${normalizedPriority}`, "success", 1500);
      await refreshLedger(selectedLedgerProjectId);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to update task priority";
      onError?.(msg);
      addToast(msg, "error", 2500);
    } finally {
      setUpdatingLedgerTaskId(0);
    }
  }, [isLoggedIn, selectedLedgerProjectId, refreshLedger, addToast, onError]);

  const toggleLedgerTaskSelection = useCallback((taskId: number, selected: boolean) => {
    setSelectedLedgerTaskIDs((prev) => {
      const next = new Set(prev);
      if (selected) {
        next.add(taskId);
      } else {
        next.delete(taskId);
      }
      return Array.from(next);
    });
  }, []);

  const handleBulkSetLedgerTaskStatus = useCallback(async (status: IndustryTaskStatus) => {
    if (!isLoggedIn || selectedLedgerProjectId <= 0) return;
    if (selectedLedgerTaskIDs.length === 0) {
      addToast("Select tasks first", "warning", 2000);
      return;
    }
    setUpdatingLedgerTasksBulk(true);
    try {
      const resp = await updateAuthIndustryTaskStatusBulk({
        task_ids: selectedLedgerTaskIDs,
        status,
      });
      addToast(`Updated ${resp.updated} tasks -> ${status}`, "success", 1800);
      await refreshLedger(selectedLedgerProjectId);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to bulk update tasks";
      onError?.(msg);
      addToast(msg, "error", 2500);
    } finally {
      setUpdatingLedgerTasksBulk(false);
    }
  }, [isLoggedIn, selectedLedgerProjectId, selectedLedgerTaskIDs, addToast, refreshLedger, onError]);

  const handleBulkSetLedgerTaskPriority = useCallback(async (priority: number) => {
    if (!isLoggedIn || selectedLedgerProjectId <= 0) return;
    if (selectedLedgerTaskIDs.length === 0) {
      addToast("Select tasks first", "warning", 2000);
      return;
    }
    const normalizedPriority = Number.isFinite(priority) ? Math.round(priority) : 0;
    setUpdatingLedgerTasksBulk(true);
    try {
      const resp = await updateAuthIndustryTaskPriorityBulk({
        task_ids: selectedLedgerTaskIDs,
        priority: normalizedPriority,
      });
      addToast(`Updated ${resp.updated} task priorities -> ${normalizedPriority}`, "success", 1800);
      await refreshLedger(selectedLedgerProjectId);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to bulk update task priorities";
      onError?.(msg);
      addToast(msg, "error", 2500);
    } finally {
      setUpdatingLedgerTasksBulk(false);
    }
  }, [isLoggedIn, selectedLedgerProjectId, selectedLedgerTaskIDs, addToast, refreshLedger, onError]);

  const handleSetLedgerJobStatus = useCallback(async (jobId: number, status: IndustryJobStatus) => {
    if (!isLoggedIn || selectedLedgerProjectId <= 0 || jobId <= 0) return;
    setUpdatingLedgerJobId(jobId);
    try {
      await updateAuthIndustryJobStatus({ job_id: jobId, status });
      addToast(t("industryLedgerJobUpdated", { id: jobId, status }), "success", 1500);
      await refreshLedger(selectedLedgerProjectId);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to update job status";
      onError?.(msg);
      addToast(msg, "error", 2500);
    } finally {
      setUpdatingLedgerJobId(0);
    }
  }, [isLoggedIn, selectedLedgerProjectId, refreshLedger, addToast, onError, t]);

  const toggleLedgerJobSelection = useCallback((jobId: number, selected: boolean) => {
    setSelectedLedgerJobIDs((prev) => {
      const next = new Set(prev);
      if (selected) {
        next.add(jobId);
      } else {
        next.delete(jobId);
      }
      return Array.from(next);
    });
  }, []);

  const handleBulkSetLedgerJobStatus = useCallback(async (status: IndustryJobStatus) => {
    if (!isLoggedIn || selectedLedgerProjectId <= 0) return;
    if (selectedLedgerJobIDs.length === 0) {
      addToast(t("industryLedgerSelectJobsFirst"), "warning", 2000);
      return;
    }
    setUpdatingLedgerJobsBulk(true);
    try {
      const resp = await updateAuthIndustryJobStatusBulk({
        job_ids: selectedLedgerJobIDs,
        status,
      });
      addToast(t("industryLedgerBulkUpdated", { count: resp.updated, status }), "success", 1800);
      await refreshLedger(selectedLedgerProjectId);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to bulk update jobs";
      onError?.(msg);
      addToast(msg, "error", 2500);
    } finally {
      setUpdatingLedgerJobsBulk(false);
    }
  }, [isLoggedIn, selectedLedgerProjectId, selectedLedgerJobIDs, addToast, refreshLedger, onError, t]);

  const handleRebalanceLedgerMaterialsFromInventory = useCallback(async () => {
    if (!isLoggedIn || selectedLedgerProjectId <= 0) return;
    setRebalancingLedgerMaterials(true);
    try {
      const locationIDs = rebalanceUseSelectedStation && selectedStationId > 0
        ? [selectedStationId]
        : [];
      const resp = await rebalanceAuthIndustryProjectMaterials(selectedLedgerProjectId, {
        scope: rebalanceInventoryScope,
        lookback_days: Math.max(1, Math.min(365, Math.round(rebalanceLookbackDays || 180))),
        strategy: rebalanceStrategy,
        warehouse_scope: rebalanceWarehouseScope,
        location_ids: locationIDs,
      });
      const s = resp.summary;
      addToast(
        `Rebalanced ${s.updated} rows · stock ${s.allocated_available.toLocaleString()} · missing ${s.remaining_missing_qty.toLocaleString()}`,
        "success",
        2400
      );
      await refreshLedger(selectedLedgerProjectId);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to rebalance materials from inventory";
      onError?.(msg);
      addToast(msg, "error", 2600);
    } finally {
      setRebalancingLedgerMaterials(false);
    }
  }, [
    isLoggedIn,
    selectedLedgerProjectId,
    rebalanceUseSelectedStation,
    selectedStationId,
    rebalanceInventoryScope,
    rebalanceLookbackDays,
    rebalanceStrategy,
    rebalanceWarehouseScope,
    addToast,
    refreshLedger,
    onError,
  ]);

  const handleSyncLedgerBlueprintPoolFromAssets = useCallback(async () => {
    if (!isLoggedIn || selectedLedgerProjectId <= 0) return;
    setSyncingLedgerBlueprintPool(true);
    try {
      const locationIDs = rebalanceUseSelectedStation && selectedStationId > 0
        ? [selectedStationId]
        : [];
      const resp = await syncAuthIndustryProjectBlueprintPool(selectedLedgerProjectId, {
        scope: rebalanceInventoryScope,
        default_bpc_runs: Math.max(1, Math.min(1000, Math.round(blueprintSyncDefaultBPCRuns || 1))),
        location_ids: locationIDs,
      });
      const s = resp.summary;
      const bpChars = s.blueprints_endpoint_characters ?? 0;
      const fallbackChars = s.assets_fallback_characters ?? 0;
      const sourceNote = fallbackChars > 0
        ? ` (bp:${bpChars} fallback:${fallbackChars})`
        : bpChars > 0
          ? ` (bp:${bpChars})`
          : "";
      addToast(
        `BP sync: ${s.blueprints_upserted} upserted (${s.blueprints_detected} detected, ${s.assets_scanned} assets scanned${sourceNote})`,
        "success",
        2600
      );
      if (Array.isArray(s.warnings) && s.warnings.length > 0) {
        addToast(s.warnings.slice(0, 2).join(" | "), "warning", 3200);
      }
      await refreshLedger(selectedLedgerProjectId);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to sync owned blueprints";
      onError?.(msg);
      addToast(msg, "error", 2600);
    } finally {
      setSyncingLedgerBlueprintPool(false);
    }
  }, [
    isLoggedIn,
    selectedLedgerProjectId,
    rebalanceUseSelectedStation,
    selectedStationId,
    rebalanceInventoryScope,
    blueprintSyncDefaultBPCRuns,
    addToast,
    refreshLedger,
    onError,
  ]);

  const handleCheckCurrentIndustryCoverage = useCallback(async () => {
    if (!isLoggedIn || !result) return;
    const needs = buildIndustryCoverageNeeds(result);
    if (needs.materials.length === 0 && needs.blueprints.length === 0) {
      addToast("No material or blueprint rows to check", "warning", 2200);
      return;
    }
    setIndustryCoverageLoading(true);
    try {
      const locationIDs = rebalanceUseSelectedStation && selectedStationId > 0
        ? [selectedStationId]
        : [];
      const resp = await getAuthIndustryCoverage({
        scope: rebalanceInventoryScope,
        default_bpc_runs: Math.max(1, Math.min(1000, Math.round(blueprintSyncDefaultBPCRuns || 1))),
        location_ids: locationIDs,
        materials: needs.materials,
        blueprints: needs.blueprints,
      });
      setIndustryCoverage(resp.coverage);
      const s = resp.summary;
      setIndustryCoverageMeta(
        `${s.scope} / ${s.characters_used}/${s.characters} chars / ${s.assets_scanned} assets / ${s.blueprint_rows_scanned} bp rows`
      );
      const c = resp.coverage.summary;
      addToast(
        `Coverage: ${c.materials_covered}/${c.materials} materials, ${c.blueprints_ready}/${c.blueprints} blueprints`,
        c.can_start_now ? "success" : "warning",
        2600
      );
      const warnings = resp.coverage.warnings ?? resp.summary.warnings ?? [];
      if (warnings.length > 0) {
        addToast(warnings.slice(0, 2).join(" | "), "warning", 3200);
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to check industry coverage";
      onError?.(msg);
      addToast(msg, "error", 2600);
    } finally {
      setIndustryCoverageLoading(false);
    }
  }, [
    isLoggedIn,
    result,
    rebalanceUseSelectedStation,
    selectedStationId,
    rebalanceInventoryScope,
    blueprintSyncDefaultBPCRuns,
    addToast,
    onError,
  ]);

  const buildAutoPlanPatch = useCallback((): IndustryPlanPatch | null => {
    if (!result || !selectedItem) {
      return null;
    }
    const topBlueprintTypeID = result.material_tree?.blueprint?.blueprint_type_id ?? 0;
    const activitySteps = result.activity_plan ?? [];
    const tasks: IndustryTaskPlanInput[] = [];
    const jobs: IndustryJobPlanInput[] = [];

    if (activitySteps.length > 0) {
      activitySteps.forEach((step, index) => {
        const targetRuns = industryStepRuns(step);
        const taskRef = -(index + 1);
        tasks.push({
          name: industryStepLabel(step),
          activity: step.activity || "manufacturing",
          product_type_id: step.product_type_id,
          target_runs: targetRuns,
          priority: 100 + index,
          status: "planned",
          constraints: {
            me,
            te,
            system_name: systemName,
            station_id: selectedStationId || 0,
            blueprint_type_id: step.blueprint_type_id || 0,
            blueprint_location_id: selectedStationId || 0,
            duration_seconds_per_run: targetRuns > 0 ? Math.round((step.time_seconds || 0) / targetRuns) : 0,
            cost_isk_per_run: targetRuns > 0 ? (step.job_cost || 0) / targetRuns : 0,
          },
        });
        jobs.push({
          task_id: taskRef,
          facility_id: selectedStationId || 0,
          activity: step.activity || "manufacturing",
          runs: targetRuns,
          duration_seconds: step.time_seconds ?? 0,
          cost_isk: step.job_cost ?? 0,
          status: "planned",
          started_at: "",
          finished_at: "",
          notes: industryCoverage ? "Coverage-aware draft from Industry analyzer" : "Draft from Industry analyzer activity plan",
        });
      });
    } else {
      const taskName = `Build ${selectedItem.type_name}`;
      tasks.push({
        name: taskName,
        activity: "manufacturing",
        product_type_id: selectedItem.type_id,
        target_runs: runs,
        priority: 100,
        status: "planned",
        constraints: {
          me,
          te,
          system_name: systemName,
          station_id: selectedStationId || 0,
          blueprint_type_id: topBlueprintTypeID || 0,
          blueprint_location_id: selectedStationId || 0,
          duration_seconds_per_run: runs > 0 ? Math.round((result.manufacturing_time ?? 0) / runs) : 0,
          cost_isk_per_run: runs > 0 ? (result.total_job_cost ?? 0) / runs : 0,
        },
      });
      jobs.push({
        task_id: -1,
        facility_id: selectedStationId || 0,
        activity: "manufacturing",
        runs,
        duration_seconds: result.manufacturing_time ?? 0,
        cost_isk: result.total_job_cost ?? 0,
        status: "planned",
        started_at: "",
        finished_at: "",
        notes: industryCoverage ? "Coverage-aware draft from Industry analyzer" : "Auto-seeded from Industry analyzer",
      });
    }

    const flatByType = new Map((result.flat_materials ?? []).map((m) => [m.type_id, m]));
    const materialSourceRows = industryCoverage?.materials?.length
      ? industryCoverage.materials
      : (result.flat_materials ?? []).map((m) => ({
          type_id: m.type_id,
          type_name: m.type_name,
          required_qty: m.quantity,
          available_qty: 0,
          missing_qty: m.quantity,
          coverage_pct: 0,
          status: "missing",
        }));
    const materials = materialSourceRows.map((m) => {
      const flat = flatByType.get(m.type_id);
      const requiredQty = Math.max(0, Math.ceil(m.required_qty ?? 0));
      const availableQty = Math.max(0, Math.min(requiredQty, Math.ceil(m.available_qty ?? 0)));
      const buyQty = Math.max(0, Math.ceil(m.missing_qty ?? Math.max(0, requiredQty - availableQty)));
      return {
        type_id: m.type_id,
        type_name: m.type_name || flat?.type_name || "",
        required_qty: requiredQty,
        available_qty: availableQty,
        buy_qty: buyQty,
        build_qty: 0,
        unit_cost_isk: flat?.unit_price ?? 0,
        source: buyQty > 0 ? "market" as const : "stock" as const,
      };
    });

    const blueprintsFromCoverage = (industryCoverage?.blueprints ?? [])
      .filter((bp) => (bp.owned_qty ?? 0) > 0 && ((bp.bpo_qty ?? 0) > 0 || (bp.available_runs ?? 0) > 0))
      .map((bp) => {
        const isBPO = (bp.bpo_qty ?? 0) > 0;
        return {
          blueprint_type_id: bp.blueprint_type_id,
          blueprint_name: bp.blueprint_name || "",
          location_id: selectedStationId || 0,
          quantity: isBPO ? Math.max(1, bp.bpo_qty || 1) : Math.max(1, bp.bpc_qty || 1),
          me: bp.best_me || me,
          te: bp.best_te || te,
          is_bpo: isBPO,
          available_runs: isBPO ? 0 : Math.max(0, bp.available_runs || 0),
        };
      });

    const fallbackBlueprintMap = new Map<number, IndustryBlueprintPoolInput>();
    if (blueprintsFromCoverage.length === 0) {
      for (const step of activitySteps) {
        if (!step.blueprint_type_id || step.blueprint_type_id <= 0) continue;
        const requiredRuns = industryStepRuns(step);
        const existing = fallbackBlueprintMap.get(step.blueprint_type_id);
        fallbackBlueprintMap.set(step.blueprint_type_id, {
          blueprint_type_id: step.blueprint_type_id,
          blueprint_name: step.blueprint_name || existing?.blueprint_name || "",
          location_id: selectedStationId || 0,
          quantity: 1,
          me,
          te,
          is_bpo: ownBlueprint,
          available_runs: ownBlueprint ? 0 : (existing?.available_runs ?? 0) + requiredRuns,
        });
      }
      if (fallbackBlueprintMap.size === 0 && topBlueprintTypeID > 0) {
        fallbackBlueprintMap.set(topBlueprintTypeID, {
          blueprint_type_id: topBlueprintTypeID,
          blueprint_name: `${selectedItem.type_name} Blueprint`,
          location_id: selectedStationId || 0,
          quantity: 1,
          me,
          te,
          is_bpo: ownBlueprint,
          available_runs: ownBlueprint ? 0 : runs,
        });
      }
    }
    const blueprints = blueprintsFromCoverage.length > 0
      ? blueprintsFromCoverage
      : Array.from(fallbackBlueprintMap.values());

    return {
      replace: replaceLedgerPlanOnApply,
      project_status: "planned",
      tasks,
      jobs,
      materials,
      blueprints,
    };
  }, [
    result,
    selectedItem,
    runs,
    me,
    te,
    systemName,
    selectedStationId,
    ownBlueprint,
    replaceLedgerPlanOnApply,
    industryCoverage,
  ]);

  const seedVisualPlanBuilderFromPatch = useCallback((patch: IndustryPlanPatch) => {
    setPlanDraftTasks(Array.isArray(patch.tasks) ? patch.tasks : []);
    setPlanDraftJobs(Array.isArray(patch.jobs) ? patch.jobs : []);
    setPlanDraftMaterials(Array.isArray(patch.materials) ? patch.materials : []);
    setPlanDraftBlueprints(Array.isArray(patch.blueprints) ? patch.blueprints : []);
  }, []);

  const seedVisualPlanBuilderFromSnapshot = useCallback((snapshot: IndustryProjectSnapshot) => {
    const tasks: IndustryTaskPlanInput[] = (snapshot.tasks ?? []).map((task) => ({
      task_id: task.id,
      parent_task_id: task.parent_task_id,
      name: task.name,
      activity: task.activity,
      product_type_id: task.product_type_id,
      target_runs: task.target_runs,
      planned_start: task.planned_start,
      planned_end: task.planned_end,
      priority: task.priority,
      status: task.status,
      constraints: task.constraints,
    }));

    const jobs: IndustryJobPlanInput[] = (snapshot.jobs ?? []).map((job) => ({
      task_id: job.task_id,
      character_id: job.character_id,
      facility_id: job.facility_id,
      activity: job.activity,
      runs: job.runs,
      duration_seconds: job.duration_seconds,
      cost_isk: job.cost_isk,
      status: job.status,
      started_at: job.started_at,
      finished_at: job.finished_at,
      external_job_id: job.external_job_id,
      notes: job.notes,
    }));

    const materials: IndustryMaterialPlanInput[] = (snapshot.materials ?? []).map((material) => ({
      task_id: material.task_id,
      type_id: material.type_id,
      type_name: material.type_name,
      required_qty: material.required_qty,
      available_qty: material.available_qty,
      buy_qty: material.buy_qty,
      build_qty: material.build_qty,
      unit_cost_isk: material.unit_cost_isk,
      source: material.source,
    }));

    const blueprints: IndustryBlueprintPoolInput[] = (snapshot.blueprints ?? []).map((bp) => ({
      blueprint_type_id: bp.blueprint_type_id,
      blueprint_name: bp.blueprint_name,
      location_id: bp.location_id,
      quantity: bp.quantity,
      me: bp.me,
      te: bp.te,
      is_bpo: bp.is_bpo,
      available_runs: bp.available_runs,
    }));

    setPlanDraftTasks(tasks);
    setPlanDraftJobs(jobs);
    setPlanDraftMaterials(materials);
    setPlanDraftBlueprints(blueprints);
  }, []);

  const buildVisualPlanPatch = useCallback((): IndustryPlanPatch => {
    const tasks = planDraftTasks
      .filter((task) => (task.name ?? "").trim().length > 0)
      .map((task) => ({
        ...task,
        name: task.name.trim(),
      }));
    const jobs = planDraftJobs.filter((job) => (job.activity ?? "").trim().length > 0);
    const materials = planDraftMaterials.filter((material) => Number(material.type_id) > 0);
    const blueprints = planDraftBlueprints.filter((bp) => Number(bp.blueprint_type_id) > 0);
    return {
      replace: replaceLedgerPlanOnApply,
      project_status: "planned",
      tasks,
      jobs,
      materials,
      blueprints,
    };
  }, [planDraftTasks, planDraftJobs, planDraftMaterials, planDraftBlueprints, replaceLedgerPlanOnApply]);

  const hasVisualPlanRows = useMemo(
    () => planDraftTasks.length > 0 || planDraftJobs.length > 0 || planDraftMaterials.length > 0 || planDraftBlueprints.length > 0,
    [planDraftTasks, planDraftJobs, planDraftMaterials, planDraftBlueprints]
  );

  const effectivePlanBuilderPageSize = Math.max(1, planBuilderPageSize);

  const planBuilderTotalPages = useMemo(
    () => ({
      tasks: Math.max(1, Math.ceil(planDraftTasks.length / effectivePlanBuilderPageSize)),
      jobs: Math.max(1, Math.ceil(planDraftJobs.length / effectivePlanBuilderPageSize)),
      materials: Math.max(1, Math.ceil(planDraftMaterials.length / effectivePlanBuilderPageSize)),
      blueprints: Math.max(1, Math.ceil(planDraftBlueprints.length / effectivePlanBuilderPageSize)),
    }),
    [
      planDraftTasks.length,
      planDraftJobs.length,
      planDraftMaterials.length,
      planDraftBlueprints.length,
      effectivePlanBuilderPageSize,
    ]
  );

  useEffect(() => {
    setPlanBuilderPage((prev) => ({
      tasks: Math.min(Math.max(1, prev.tasks || 1), planBuilderTotalPages.tasks),
      jobs: Math.min(Math.max(1, prev.jobs || 1), planBuilderTotalPages.jobs),
      materials: Math.min(Math.max(1, prev.materials || 1), planBuilderTotalPages.materials),
      blueprints: Math.min(Math.max(1, prev.blueprints || 1), planBuilderTotalPages.blueprints),
    }));
  }, [planBuilderTotalPages]);

  const taskPageStart = planBuilderCompactMode ? (planBuilderPage.tasks - 1) * effectivePlanBuilderPageSize : 0;
  const jobPageStart = planBuilderCompactMode ? (planBuilderPage.jobs - 1) * effectivePlanBuilderPageSize : 0;
  const materialPageStart = planBuilderCompactMode ? (planBuilderPage.materials - 1) * effectivePlanBuilderPageSize : 0;
  const blueprintPageStart = planBuilderCompactMode ? (planBuilderPage.blueprints - 1) * effectivePlanBuilderPageSize : 0;

  const visiblePlanDraftTasks = useMemo(
    () => (planBuilderCompactMode
      ? planDraftTasks.slice(taskPageStart, taskPageStart + effectivePlanBuilderPageSize)
      : planDraftTasks),
    [planBuilderCompactMode, planDraftTasks, taskPageStart, effectivePlanBuilderPageSize]
  );

  const visiblePlanDraftJobs = useMemo(
    () => (planBuilderCompactMode
      ? planDraftJobs.slice(jobPageStart, jobPageStart + effectivePlanBuilderPageSize)
      : planDraftJobs),
    [planBuilderCompactMode, planDraftJobs, jobPageStart, effectivePlanBuilderPageSize]
  );

  const visiblePlanDraftMaterials = useMemo(
    () => (planBuilderCompactMode
      ? planDraftMaterials.slice(materialPageStart, materialPageStart + effectivePlanBuilderPageSize)
      : planDraftMaterials),
    [planBuilderCompactMode, planDraftMaterials, materialPageStart, effectivePlanBuilderPageSize]
  );

  const visiblePlanDraftBlueprints = useMemo(
    () => (planBuilderCompactMode
      ? planDraftBlueprints.slice(blueprintPageStart, blueprintPageStart + effectivePlanBuilderPageSize)
      : planDraftBlueprints),
    [planBuilderCompactMode, planDraftBlueprints, blueprintPageStart, effectivePlanBuilderPageSize]
  );

  const planTaskBlueprintOptions = useMemo(() => (
    planDraftBlueprints
      .map((bp, idx) => {
        const blueprintTypeID = Number(bp.blueprint_type_id) || 0;
        const blueprintLocationID = Number(bp.location_id) || 0;
        if (blueprintTypeID <= 0) return null;
        const meValue = Number(bp.me) || 0;
        const teValue = Number(bp.te) || 0;
        const runsValue = Number(bp.available_runs) || 0;
        const qtyValue = Number(bp.quantity) || 0;
        const name = (bp.blueprint_name ?? "").trim();
        const labelName = name || `BP ${blueprintTypeID}`;
        const label = `${labelName} [${blueprintTypeID}] @${blueprintLocationID || "any"} ${bp.is_bpo ? "BPO" : "BPC"} runs:${runsValue} qty:${qtyValue} ME:${meValue} TE:${teValue}`;
        return {
          value: `${idx}:${blueprintTypeID}:${blueprintLocationID}`,
          label,
          blueprintTypeID,
          blueprintLocationID,
          me: meValue,
          te: teValue,
        };
      })
      .filter((row): row is {
        value: string;
        label: string;
        blueprintTypeID: number;
        blueprintLocationID: number;
        me: number;
        te: number;
      } => row !== null)
  ), [planDraftBlueprints]);

  const planTaskBlueprintOptionByPair = useMemo(() => {
    const byPair = new Map<string, string>();
    for (const option of planTaskBlueprintOptions) {
      const key = `${option.blueprintTypeID}:${option.blueprintLocationID}`;
      if (!byPair.has(key)) {
        byPair.set(key, option.value);
      }
    }
    return byPair;
  }, [planTaskBlueprintOptions]);

  const planTaskBlueprintOptionByType = useMemo(() => {
    const byType = new Map<number, string>();
    for (const option of planTaskBlueprintOptions) {
      if (!byType.has(option.blueprintTypeID)) {
        byType.set(option.blueprintTypeID, option.value);
      }
    }
    return byType;
  }, [planTaskBlueprintOptions]);

  const visualTaskBlueprintBindingStats = useMemo(() => {
    let none = 0;
    let exact = 0;
    let fallback = 0;
    let missing = 0;
    for (const task of planDraftTasks) {
      const bpTypeID = taskConstraintNumber(task.constraints, "blueprint_type_id");
      const bpLocationID = taskConstraintNumber(task.constraints, "blueprint_location_id");
      if (bpTypeID <= 0) {
        none++;
        continue;
      }
      if (planTaskBlueprintOptionByPair.has(`${bpTypeID}:${bpLocationID}`)) {
        exact++;
        continue;
      }
      if (planTaskBlueprintOptionByType.has(bpTypeID)) {
        fallback++;
        continue;
      }
      missing++;
    }
    return {
      total: planDraftTasks.length,
      none,
      exact,
      fallback,
      missing,
    };
  }, [planDraftTasks, planTaskBlueprintOptionByPair, planTaskBlueprintOptionByType]);

  const strictBlueprintApplyBlocked = useMemo(() => (
    strictBlueprintBindingMode &&
    useVisualPlanBuilder &&
    hasVisualPlanRows &&
    visualTaskBlueprintBindingStats.missing > 0
  ), [
    strictBlueprintBindingMode,
    useVisualPlanBuilder,
    hasVisualPlanRows,
    visualTaskBlueprintBindingStats.missing,
  ]);

  const plannerWarningSourceLabel = useCallback((source: IndustryPlannerWarningSource): string => {
    switch (source) {
      case "preview":
        return t("industryLedgerWarningSourcePreview");
      case "apply":
        return t("industryLedgerWarningSourceApply");
      case "gate":
        return t("industryLedgerWarningSourceGate");
      default:
        return source;
    }
  }, [t]);

  const taskStatusBoard = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const task of ledgerSnapshot?.tasks ?? []) {
      const key = String(task.status || "planned");
      counts[key] = (counts[key] || 0) + 1;
    }
    return counts;
  }, [ledgerSnapshot]);

  const jobStatusBoard = useMemo(() => {
    const counts: Record<string, number> = {};
    if (ledgerSnapshot && ledgerSnapshot.jobs.length > 0) {
      for (const job of ledgerSnapshot.jobs) {
        const key = String(job.status || "planned");
        counts[key] = (counts[key] || 0) + 1;
      }
      return counts;
    }
    if (ledgerData && ledgerData.entries.length > 0) {
      for (const row of ledgerData.entries) {
        const key = String(row.status || "planned");
        counts[key] = (counts[key] || 0) + 1;
      }
      return counts;
    }
    if (ledgerData) {
      counts.planned = ledgerData.planned || 0;
      counts.active = ledgerData.active || 0;
      counts.completed = ledgerData.completed || 0;
      counts.failed = ledgerData.failed || 0;
      counts.cancelled = ledgerData.cancelled || 0;
    }
    return counts;
  }, [ledgerSnapshot, ledgerData]);

  const taskStatusTotal = useMemo(
    () => Object.values(taskStatusBoard).reduce((sum, value) => sum + (Number(value) || 0), 0),
    [taskStatusBoard]
  );
  const taskStatusDone = (taskStatusBoard.completed || 0) + (taskStatusBoard.cancelled || 0);
  const taskStatusDonePct = taskStatusTotal > 0
    ? Math.round((taskStatusDone / taskStatusTotal) * 100)
    : 0;

  const jobStatusTotal = useMemo(
    () => Object.values(jobStatusBoard).reduce((sum, value) => sum + (Number(value) || 0), 0),
    [jobStatusBoard]
  );
  const jobStatusDone = (jobStatusBoard.completed || 0) + (jobStatusBoard.cancelled || 0) + (jobStatusBoard.failed || 0);
  const jobStatusDonePct = jobStatusTotal > 0
    ? Math.round((jobStatusDone / jobStatusTotal) * 100)
    : 0;

  const materialCoverageTotals = useMemo(() => {
    let required = 0;
    let stock = 0;
    let buy = 0;
    let build = 0;
    let missing = 0;
    for (const row of ledgerSnapshot?.material_diff ?? []) {
      required += Number(row.required_qty) || 0;
      stock += Number(row.available_qty) || 0;
      buy += Number(row.buy_qty) || 0;
      build += Number(row.build_qty) || 0;
      missing += Number(row.missing_qty) || 0;
    }
    return { required, stock, buy, build, missing };
  }, [ledgerSnapshot]);
  const activeJobCount = jobStatusBoard.active || 0;
  const hasPlanSeedSource = Boolean(result && selectedItem);

  const taskDependencyBoard = useMemo<IndustryTaskDependencyBoard>(() => {
    const tasks = ledgerSnapshot?.tasks ?? [];
    const byID = new Map<number, typeof tasks[number]>();
    for (const task of tasks) {
      byID.set(task.id, task);
    }

    const childrenByParent = new Map<number, number[]>();
    const indegreeByID = new Map<number, number>();
    const durationSecByID = new Map<number, number>();
    const predecessorByID = new Map<number, number>();
    const parentByTaskID: Record<number, number> = {};
    const parentMissingByTaskID: Record<number, boolean> = {};
    const rows: Array<{
      child_id: number;
      child_name: string;
      child_status: string;
      parent_id: number;
      parent_name: string;
      parent_status: string;
      parent_missing: boolean;
    }> = [];
    const orphanTaskIDs = new Set<number>();
    let edgeCount = 0;
    let selfLinkCount = 0;

    for (const task of tasks) {
      indegreeByID.set(task.id, 0);
      const startMs = new Date(task.planned_start || "").getTime();
      const endMs = new Date(task.planned_end || "").getTime();
      const duration = Number.isFinite(startMs) && Number.isFinite(endMs) && endMs > startMs
        ? Math.round((endMs - startMs) / 1000)
        : 0;
      durationSecByID.set(task.id, duration);
    }

    for (const task of tasks) {
      const parentID = Number(task.parent_task_id) || 0;
      if (parentID <= 0) {
        continue;
      }
      if (parentID === task.id) {
        selfLinkCount++;
        rows.push({
          child_id: task.id,
          child_name: task.name || `Task ${task.id}`,
          child_status: task.status || "planned",
          parent_id: parentID,
          parent_name: "self",
          parent_status: task.status || "planned",
          parent_missing: true,
        });
        parentByTaskID[task.id] = parentID;
        parentMissingByTaskID[task.id] = true;
        continue;
      }
      const parent = byID.get(parentID);
      if (!parent) {
        orphanTaskIDs.add(task.id);
        rows.push({
          child_id: task.id,
          child_name: task.name || `Task ${task.id}`,
          child_status: task.status || "planned",
          parent_id: parentID,
          parent_name: `Missing ${parentID}`,
          parent_status: "missing",
          parent_missing: true,
        });
        parentByTaskID[task.id] = parentID;
        parentMissingByTaskID[task.id] = true;
        continue;
      }
      const children = childrenByParent.get(parentID) ?? [];
      children.push(task.id);
      childrenByParent.set(parentID, children);
      indegreeByID.set(task.id, (indegreeByID.get(task.id) || 0) + 1);
      edgeCount++;
      parentByTaskID[task.id] = parentID;
      rows.push({
        child_id: task.id,
        child_name: task.name || `Task ${task.id}`,
        child_status: task.status || "planned",
        parent_id: parent.id,
        parent_name: parent.name || `Task ${parent.id}`,
        parent_status: parent.status || "planned",
        parent_missing: false,
      });
    }

    const rootCount = tasks.filter((task) => (indegreeByID.get(task.id) || 0) === 0).length;
    const leafCount = tasks.filter((task) => !(childrenByParent.get(task.id)?.length)).length;

    const indegreeWorking = new Map(indegreeByID);
    const queue: number[] = [];
    const depthByID = new Map<number, number>();
    const pathSecByID = new Map<number, number>();
    for (const task of tasks) {
      if ((indegreeWorking.get(task.id) || 0) === 0) {
        queue.push(task.id);
        depthByID.set(task.id, 1);
        pathSecByID.set(task.id, durationSecByID.get(task.id) || 0);
      }
    }
    let processed = 0;
    while (queue.length > 0) {
      const currentID = queue.shift() || 0;
      if (!currentID) continue;
      processed++;
      const currentDepth = depthByID.get(currentID) || 1;
      const currentPathSec = pathSecByID.get(currentID) || (durationSecByID.get(currentID) || 0);
      const children = childrenByParent.get(currentID) ?? [];
      for (const childID of children) {
        const nextDepth = Math.max(depthByID.get(childID) || 1, currentDepth + 1);
        depthByID.set(childID, nextDepth);
        const childDuration = durationSecByID.get(childID) || 0;
        const existingPath = pathSecByID.get(childID) || childDuration;
        const candidatePath = currentPathSec + childDuration;
        if (candidatePath > existingPath || (candidatePath === existingPath && !predecessorByID.has(childID))) {
          pathSecByID.set(childID, candidatePath);
          predecessorByID.set(childID, currentID);
        }
        const nextIn = (indegreeWorking.get(childID) || 0) - 1;
        indegreeWorking.set(childID, nextIn);
        if (nextIn === 0) {
          queue.push(childID);
        }
      }
    }

    const maxDepth = depthByID.size > 0
      ? Math.max(...Array.from(depthByID.values()))
      : 0;
    const criticalPathSec = pathSecByID.size > 0
      ? Math.max(...Array.from(pathSecByID.values()))
      : 0;
    let criticalEndTaskID = 0;
    for (const [taskID, pathSec] of pathSecByID.entries()) {
      if (pathSec === criticalPathSec) {
        criticalEndTaskID = taskID;
      }
    }
    const criticalTaskIDSet = new Set<number>();
    let walkID = criticalEndTaskID;
    while (walkID > 0 && !criticalTaskIDSet.has(walkID)) {
      criticalTaskIDSet.add(walkID);
      walkID = predecessorByID.get(walkID) || 0;
    }
    const cycleCount = Math.max(0, tasks.length - processed);

    rows.sort((a, b) => {
      if (a.parent_missing !== b.parent_missing) return a.parent_missing ? -1 : 1;
      return a.child_id - b.child_id;
    });

    return {
      total_tasks: tasks.length,
      total_edges: edgeCount,
      roots: rootCount,
      leaves: leafCount,
      max_depth: maxDepth,
      critical_path_sec: criticalPathSec,
      orphans: orphanTaskIDs.size,
      cycles: cycleCount,
      self_links: selfLinkCount,
      depth_by_task: Object.fromEntries(depthByID.entries()) as Record<number, number>,
      parent_by_task: parentByTaskID,
      parent_missing_by_task: parentMissingByTaskID,
      critical_task_ids: criticalTaskIDSet,
      rows,
    };
  }, [ledgerSnapshot]);

  const togglePlanBuilderSection = useCallback((section: PlanBuilderSection) => {
    setPlanBuilderCollapsed((prev) => ({ ...prev, [section]: !prev[section] }));
  }, []);

  const changePlanBuilderPage = useCallback((section: PlanBuilderSection, nextPage: number) => {
    setPlanBuilderPage((prev) => ({ ...prev, [section]: Math.max(1, nextPage) }));
  }, []);

  const handleGeneratePlanDraft = useCallback(() => {
    const patch = buildAutoPlanPatch();
    if (!patch) {
      addToast(t("industryLedgerRunAnalysisFirst"), "warning", 2200);
      return;
    }
    seedVisualPlanBuilderFromPatch(patch);
    setUseVisualPlanBuilder(true);
    setLastLedgerPlanPreview(null);
    setLastLedgerPlanPreviewPatch(null);
    addToast(t("industryLedgerBuilderSeeded"), "success", 1600);
  }, [buildAutoPlanPatch, seedVisualPlanBuilderFromPatch, addToast, t]);

  const handleSeedCurrentIndustryPlanFromAnalysis = useCallback(() => {
    handleGeneratePlanDraft();
    setIndustryInnerTab("jobs");
    setJobsWorkspaceTab("planning");
  }, [handleGeneratePlanDraft]);

  const buildLedgerPlanPatchToSend = useCallback((): IndustryPlanPatch | null => {
    let patch: IndustryPlanPatch | null = null;
    if (useVisualPlanBuilder && hasVisualPlanRows) {
      patch = buildVisualPlanPatch();
    } else {
      patch = buildAutoPlanPatch();
    }
    if (!patch) {
      return null;
    }
    const schedulerPatch: IndustryPlanSchedulerInput = {
      enabled: enablePlanScheduler,
      slot_count: Math.max(1, schedulerSlotCount),
      max_job_runs: Math.max(1, schedulerMaxRunsPerJob),
      max_job_duration_seconds: Math.max(1, Math.round(schedulerMaxDurationHours * 3600)),
      window_days: 30,
      queue_status: schedulerQueueStatus,
    };
    return {
      replace: patch.replace ?? replaceLedgerPlanOnApply,
      project_status: patch.project_status ?? "planned",
      tasks: Array.isArray(patch.tasks) ? patch.tasks : [],
      jobs: Array.isArray(patch.jobs) ? patch.jobs : [],
      materials: Array.isArray(patch.materials) ? patch.materials : [],
      blueprints: Array.isArray(patch.blueprints) ? patch.blueprints : [],
      scheduler: schedulerPatch,
    };
  }, [
    useVisualPlanBuilder,
    hasVisualPlanRows,
    buildVisualPlanPatch,
    buildAutoPlanPatch,
    enablePlanScheduler,
    schedulerSlotCount,
    schedulerMaxRunsPerJob,
    schedulerMaxDurationHours,
    schedulerQueueStatus,
    replaceLedgerPlanOnApply,
  ]);

  const currentLedgerPlanPatchSignature = useMemo(() => {
    const patch = buildLedgerPlanPatchToSend();
    return planPatchSignature(patch);
  }, [buildLedgerPlanPatchToSend]);

  const lastLedgerPreviewPatchSignature = useMemo(
    () => planPatchSignature(lastLedgerPlanPreviewPatch),
    [lastLedgerPlanPreviewPatch]
  );

  const isLastLedgerPreviewStale = useMemo(() => (
    Boolean(
      lastLedgerPlanPreview &&
      lastLedgerPreviewPatchSignature &&
      currentLedgerPlanPatchSignature &&
      lastLedgerPreviewPatchSignature !== currentLedgerPlanPatchSignature
    )
  ), [
    lastLedgerPlanPreview,
    lastLedgerPreviewPatchSignature,
    currentLedgerPlanPatchSignature,
  ]);

  const visibleLedgerTaskIDs = useMemo(
    () => (ledgerSnapshot?.tasks ?? []).map((task) => task.id),
    [ledgerSnapshot]
  );

  const selectedLedgerTaskIDSet = useMemo(
    () => new Set(selectedLedgerTaskIDs),
    [selectedLedgerTaskIDs]
  );

  const allVisibleLedgerTasksSelected = useMemo(() => (
    visibleLedgerTaskIDs.length > 0 &&
    visibleLedgerTaskIDs.every((taskID) => selectedLedgerTaskIDSet.has(taskID))
  ), [visibleLedgerTaskIDs, selectedLedgerTaskIDSet]);

  const handleSelectAllVisibleLedgerTasks = useCallback((selected: boolean) => {
    if (!selected) {
      setSelectedLedgerTaskIDs([]);
      return;
    }
    setSelectedLedgerTaskIDs(visibleLedgerTaskIDs);
  }, [visibleLedgerTaskIDs]);

  const visibleLedgerJobIDs = useMemo(
    () => (ledgerData?.entries ?? []).map((entry) => entry.job_id),
    [ledgerData]
  );

  const selectedLedgerJobIDSet = useMemo(
    () => new Set(selectedLedgerJobIDs),
    [selectedLedgerJobIDs]
  );

  const allVisibleLedgerJobsSelected = useMemo(() => (
    visibleLedgerJobIDs.length > 0 &&
    visibleLedgerJobIDs.every((jobID) => selectedLedgerJobIDSet.has(jobID))
  ), [visibleLedgerJobIDs, selectedLedgerJobIDSet]);

  const handleSelectAllVisibleLedgerJobs = useCallback((selected: boolean) => {
    if (!selected) {
      setSelectedLedgerJobIDs([]);
      return;
    }
    setSelectedLedgerJobIDs(visibleLedgerJobIDs);
  }, [visibleLedgerJobIDs]);

  const handleLoadLedgerSnapshotToBuilder = useCallback(() => {
    if (!ledgerSnapshot) {
      addToast(t("industryLedgerNoSnapshot"), "warning", 2000);
      return;
    }
    seedVisualPlanBuilderFromSnapshot(ledgerSnapshot);
    setUseVisualPlanBuilder(true);
    setLastLedgerPlanPreview(null);
    setLastLedgerPlanPreviewPatch(null);
    addToast(t("industryLedgerSnapshotLoaded"), "success", 1800);
  }, [ledgerSnapshot, seedVisualPlanBuilderFromSnapshot, addToast, t]);

  const handlePreviewCurrentAnalysisToLedgerPlan = useCallback(async () => {
    if (!isLoggedIn) return;
    if (selectedLedgerProjectId <= 0) {
      addToast(t("industryLedgerSelectProjectFirst"), "warning", 2000);
      return;
    }
    const patchToSend = buildLedgerPlanPatchToSend();
    if (!patchToSend) {
      addToast(t("industryLedgerRunAnalysisFirst"), "warning", 2200);
      return;
    }
    setPreviewingLedgerPlan(true);
    try {
      const preview = await previewAuthIndustryProjectPlan(selectedLedgerProjectId, patchToSend);
      setLastLedgerPlanPreview(preview);
      setLastLedgerPlanPreviewPatch(patchToSend);
      const s = preview.summary;
      const schedulerBit = s.scheduler_applied
        ? ` split:${s.jobs_split_from ?? 0}->${s.jobs_planned_total ?? s.jobs_inserted}`
        : "";
      addToast(`${t("industryLedgerPreviewReady")}: tasks:${s.tasks_inserted} jobs:${s.jobs_inserted}${schedulerBit}`, "success", 2000);
      const previewWarnings = [
        ...(Array.isArray(preview.warnings) ? preview.warnings : []),
        ...(Array.isArray(s.warnings) ? s.warnings : []),
      ];
      if (previewWarnings.length > 0) {
        pushPlannerWarnings("preview", previewWarnings);
        addToast(previewWarnings.slice(0, 2).join(" | "), "warning", 3600);
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to preview plan";
      onError?.(msg);
      addToast(msg, "error", 2600);
    } finally {
      setPreviewingLedgerPlan(false);
    }
  }, [
    isLoggedIn,
    selectedLedgerProjectId,
    buildLedgerPlanPatchToSend,
    addToast,
    onError,
    pushPlannerWarnings,
    t,
  ]);

  const handleApplyCurrentAnalysisToLedgerPlan = useCallback(async (bypassStrictGate = false) => {
    if (!isLoggedIn) return;
    if (selectedLedgerProjectId <= 0) {
      addToast(t("industryLedgerSelectProjectFirst"), "warning", 2000);
      return;
    }

    const patchToSend = buildLedgerPlanPatchToSend();
    if (!patchToSend) {
      addToast(t("industryLedgerRunAnalysisFirst"), "warning", 2200);
      return;
    }
    if (strictBlueprintApplyBlocked && !bypassStrictGate) {
      const strictGateMessage = "Strict BP gate: fix missing bindings before Apply";
      pushPlannerWarnings("gate", strictGateMessage);
      addToast(strictGateMessage, "warning", 2600);
      return;
    }

    const patchForApply: IndustryPlanPatch = bypassStrictGate
      ? { ...patchToSend, strict_bp_bypass: true }
      : patchToSend;

    setApplyingLedgerPlan(true);
    try {
      if (bypassStrictGate) {
        pushPlannerWarnings("gate", "Strict BP gate bypass requested for Apply Current");
      }
      const resp = await planAuthIndustryProject(selectedLedgerProjectId, patchForApply);
      const summary = resp.summary;
      const schedulerBit = summary.scheduler_applied
        ? ` split:${summary.jobs_split_from ?? 0}->${summary.jobs_planned_total ?? summary.jobs_inserted}`
        : "";
      const summaryText = `tasks:${summary.tasks_inserted} jobs:${summary.jobs_inserted} mats:${summary.materials_upserted} bp:${summary.blueprints_upserted}${schedulerBit}`;
      setLastLedgerPlanSummary(summaryText);
      if (Array.isArray(summary.warnings) && summary.warnings.length > 0) {
        pushPlannerWarnings("apply", summary.warnings);
        addToast(summary.warnings.slice(0, 2).join(" | "), "warning", 3600);
      }
      setLastLedgerPlanPreview(null);
      setLastLedgerPlanPreviewPatch(null);
      addToast(t("industryLedgerPlanApplied"), "success", 1800);
      await refreshLedger(selectedLedgerProjectId);
      await refreshLedgerProjects(selectedLedgerProjectId);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to apply plan";
      onError?.(msg);
      addToast(msg, "error", 2600);
    } finally {
      setApplyingLedgerPlan(false);
    }
  }, [
    isLoggedIn,
    selectedLedgerProjectId,
    buildLedgerPlanPatchToSend,
    addToast,
    onError,
    refreshLedger,
    refreshLedgerProjects,
    pushPlannerWarnings,
    strictBlueprintApplyBlocked,
    t,
  ]);

  const handleApplyLastPreviewToLedgerPlan = useCallback(async (bypassStrictGate = false) => {
    if (!isLoggedIn) return;
    if (selectedLedgerProjectId <= 0) {
      addToast(t("industryLedgerSelectProjectFirst"), "warning", 2000);
      return;
    }
    if (!lastLedgerPlanPreviewPatch) {
      addToast(t("industryLedgerRunPreviewFirst"), "warning", 2000);
      return;
    }
    if (strictBlueprintApplyBlocked && !bypassStrictGate) {
      const strictGateMessage = "Strict BP gate: fix missing bindings before Apply";
      pushPlannerWarnings("gate", strictGateMessage);
      addToast(strictGateMessage, "warning", 2600);
      return;
    }

    const patchForApply: IndustryPlanPatch = bypassStrictGate
      ? { ...lastLedgerPlanPreviewPatch, strict_bp_bypass: true }
      : lastLedgerPlanPreviewPatch;

    setApplyingLedgerPlan(true);
    try {
      if (bypassStrictGate) {
        pushPlannerWarnings("gate", "Strict BP gate bypass requested for Apply Preview");
      }
      const resp = await planAuthIndustryProject(selectedLedgerProjectId, patchForApply);
      const summary = resp.summary;
      const schedulerBit = summary.scheduler_applied
        ? ` split:${summary.jobs_split_from ?? 0}->${summary.jobs_planned_total ?? summary.jobs_inserted}`
        : "";
      const summaryText = `tasks:${summary.tasks_inserted} jobs:${summary.jobs_inserted} mats:${summary.materials_upserted} bp:${summary.blueprints_upserted}${schedulerBit}`;
      setLastLedgerPlanSummary(summaryText);
      if (Array.isArray(summary.warnings) && summary.warnings.length > 0) {
        pushPlannerWarnings("apply", summary.warnings);
        addToast(summary.warnings.slice(0, 2).join(" | "), "warning", 3600);
      }
      setLastLedgerPlanPreview(null);
      setLastLedgerPlanPreviewPatch(null);
      addToast(t("industryLedgerPreviewApplied"), "success", 1800);
      await refreshLedger(selectedLedgerProjectId);
      await refreshLedgerProjects(selectedLedgerProjectId);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Failed to apply preview";
      onError?.(msg);
      addToast(msg, "error", 2600);
    } finally {
      setApplyingLedgerPlan(false);
    }
  }, [
    isLoggedIn,
    selectedLedgerProjectId,
    lastLedgerPlanPreviewPatch,
    addToast,
    onError,
    refreshLedger,
    refreshLedgerProjects,
    pushPlannerWarnings,
    strictBlueprintApplyBlocked,
    t,
  ]);

  const addVisualTaskRow = useCallback(() => {
    const topBlueprintTypeID = result?.material_tree?.blueprint?.blueprint_type_id ?? 0;
    setPlanDraftTasks((prev) => [
      ...prev,
      {
        name: selectedItem ? `Build ${selectedItem.type_name}` : "New task",
        activity: "manufacturing",
        product_type_id: selectedItem?.type_id ?? 0,
        target_runs: runs,
        priority: 100,
        status: "planned",
        constraints: {
          me,
          te,
          station_id: selectedStationId || 0,
          blueprint_type_id: topBlueprintTypeID || 0,
          blueprint_location_id: selectedStationId || 0,
        },
      },
    ]);
  }, [selectedItem, runs, result, me, te, selectedStationId]);

  const addVisualJobRow = useCallback(() => {
    setPlanDraftJobs((prev) => [
      ...prev,
      {
        activity: "manufacturing",
        runs,
        duration_seconds: result?.manufacturing_time ?? 0,
        cost_isk: result?.total_job_cost ?? 0,
        status: "planned",
        notes: "",
      },
    ]);
  }, [runs, result]);

  const addVisualMaterialRow = useCallback(() => {
    setPlanDraftMaterials((prev) => [
      ...prev,
      {
        type_id: 0,
        type_name: "",
        required_qty: 0,
        available_qty: 0,
        buy_qty: 0,
        build_qty: 0,
        unit_cost_isk: 0,
        source: "market",
      },
    ]);
  }, []);

  const addVisualBlueprintRow = useCallback(() => {
    setPlanDraftBlueprints((prev) => [
      ...prev,
      {
        blueprint_type_id: 0,
        blueprint_name: "",
        location_id: selectedStationId || 0,
        quantity: 1,
        me,
        te,
        is_bpo: ownBlueprint,
        available_runs: 0,
      },
    ]);
  }, [selectedStationId, me, te, ownBlueprint]);

  const updateVisualTaskRow = useCallback((index: number, next: Partial<IndustryTaskPlanInput>) => {
    setPlanDraftTasks((prev) => prev.map((row, i) => (i === index ? { ...row, ...next } : row)));
  }, []);

  const updateVisualTaskConstraints = useCallback((index: number, patch: Record<string, number>) => {
    setPlanDraftTasks((prev) =>
      prev.map((row, i) => {
        if (i !== index) return row;
        const constraints = taskConstraintRecord(row.constraints);
        for (const [key, value] of Object.entries(patch)) {
          constraints[key] = Number.isFinite(value) ? value : 0;
        }
        return { ...row, constraints };
      })
    );
  }, []);

  const updateVisualTaskConstraint = useCallback((index: number, key: string, value: number) => {
    const normalized = Number.isFinite(value) ? value : 0;
    updateVisualTaskConstraints(index, { [key]: normalized });
  }, [updateVisualTaskConstraints]);

  const autoFixVisualTaskBlueprintBindings = useCallback(() => {
    if (planDraftTasks.length === 0) {
      addToast("No task rows to fix", "warning", 1800);
      return;
    }
    if (planTaskBlueprintOptions.length === 0) {
      addToast("Add blueprint pool rows first", "warning", 2200);
      return;
    }

    const optionsByType = new Map<number, typeof planTaskBlueprintOptions>();
    for (const option of planTaskBlueprintOptions) {
      const current = optionsByType.get(option.blueprintTypeID) ?? [];
      current.push(option);
      optionsByType.set(option.blueprintTypeID, current);
    }

    let fixed = 0;
    let unresolved = 0;
    const nextTasks = planDraftTasks.map((task) => {
      const bpTypeID = taskConstraintNumber(task.constraints, "blueprint_type_id");
      if (bpTypeID <= 0) {
        return task;
      }
      const candidates = optionsByType.get(bpTypeID) ?? [];
      if (candidates.length === 0) {
        unresolved++;
        return task;
      }

      const bpLocationID = taskConstraintNumber(task.constraints, "blueprint_location_id");
      const stationID = taskConstraintNumber(task.constraints, "station_id");

      const hasExact = candidates.some((option) => option.blueprintLocationID === bpLocationID);
      if (hasExact) {
        return task;
      }

      const selected =
        (stationID > 0 ? candidates.find((option) => option.blueprintLocationID === stationID) : undefined) ??
        (bpLocationID > 0 ? candidates.find((option) => option.blueprintLocationID === bpLocationID) : undefined) ??
        candidates.find((option) => option.blueprintLocationID === 0) ??
        candidates[0];

      if (!selected) {
        unresolved++;
        return task;
      }

      const constraints = taskConstraintRecord(task.constraints);
      constraints.blueprint_type_id = selected.blueprintTypeID;
      constraints.blueprint_location_id = selected.blueprintLocationID;
      constraints.me = selected.me;
      constraints.te = selected.te;
      if ((!constraints.station_id || Number(constraints.station_id) <= 0) && selected.blueprintLocationID > 0) {
        constraints.station_id = selected.blueprintLocationID;
      }
      fixed++;
      return {
        ...task,
        constraints,
      };
    });

    if (fixed > 0) {
      setPlanDraftTasks(nextTasks);
    }

    if (fixed > 0 && unresolved > 0) {
      addToast(`BP bindings fixed: ${fixed}, unresolved: ${unresolved}`, "warning", 3000);
      return;
    }
    if (fixed > 0) {
      addToast(`BP bindings fixed: ${fixed}`, "success", 2200);
      return;
    }
    if (unresolved > 0) {
      addToast(`No matching pool rows for ${unresolved} bindings`, "warning", 2600);
      return;
    }
    addToast("BP bindings are already aligned", "success", 1800);
  }, [planDraftTasks, planTaskBlueprintOptions, addToast]);

  const updateVisualJobRow = useCallback((index: number, next: Partial<IndustryJobPlanInput>) => {
    setPlanDraftJobs((prev) => prev.map((row, i) => (i === index ? { ...row, ...next } : row)));
  }, []);

  const updateVisualMaterialRow = useCallback((index: number, next: Partial<IndustryMaterialPlanInput>) => {
    setPlanDraftMaterials((prev) => prev.map((row, i) => (i === index ? { ...row, ...next } : row)));
  }, []);

  const updateVisualBlueprintRow = useCallback((index: number, next: Partial<IndustryBlueprintPoolInput>) => {
    setPlanDraftBlueprints((prev) =>
      prev.map((row, i) => {
        if (i !== index) return row;
        const merged: IndustryBlueprintPoolInput = { ...row, ...next };
        if (merged.is_bpo) {
          merged.available_runs = 0;
        } else if ((merged.available_runs ?? 0) < 0) {
          merged.available_runs = 0;
        }
        return merged;
      })
    );
  }, []);

  const removeVisualTaskRow = useCallback((index: number) => {
    setPlanDraftTasks((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const removeVisualJobRow = useCallback((index: number) => {
    setPlanDraftJobs((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const removeVisualMaterialRow = useCallback((index: number) => {
    setPlanDraftMaterials((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const removeVisualBlueprintRow = useCallback((index: number) => {
    setPlanDraftBlueprints((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const clearVisualPlanBuilder = useCallback(() => {
    setPlanDraftTasks([]);
    setPlanDraftJobs([]);
    setPlanDraftMaterials([]);
    setPlanDraftBlueprints([]);
  }, []);

  // Search handler with debounce
  const handleSearch = useCallback((query: string) => {
    setSearchQuery(query);
    setHighlightedIndex(-1);
    // Clear previous selection when user types new query
    setSelectedItem(null);
    clearTimeout(searchTimeoutRef.current);
    searchAbortRef.current?.abort();
    searchRequestSeqRef.current += 1;
    const reqSeq = searchRequestSeqRef.current;

    if (!query.trim()) {
      setSearchResults([]);
      setShowDropdown(false);
      setSearching(false);
      return;
    }

    searchTimeoutRef.current = setTimeout(async () => {
      if (reqSeq !== searchRequestSeqRef.current) return;
      const controller = new AbortController();
      searchAbortRef.current = controller;
      setSearching(true);
      try {
        const results = await searchBuildableItems(query, 30, controller.signal);
        if (reqSeq !== searchRequestSeqRef.current) return;
        // Ensure we always have an array (API might return null)
        const safeResults = results ?? [];
        setSearchResults(safeResults);
        setShowDropdown(safeResults.length > 0);
        setHighlightedIndex(safeResults.length > 0 ? 0 : -1);
      } catch (e) {
        if (reqSeq !== searchRequestSeqRef.current) return;
        if (e instanceof Error && e.name === "AbortError") return;
        console.error("Search error:", e);
        setSearchResults([]);
        setShowDropdown(false);
      } finally {
        if (reqSeq === searchRequestSeqRef.current) {
          setSearching(false);
        }
      }
    }, 200); // Faster debounce for better UX
  }, []);

  // Select item
  const handleSelectItem = useCallback((item: BuildableItem) => {
    setSelectedItem(item);
    setSearchQuery(item.type_name);
    setShowDropdown(false);
    setHighlightedIndex(-1);
    setResult(null);
    setIndustryCoverage(null);
    setIndustryCoverageMeta("");
  }, []);

  // Keyboard navigation
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (!showDropdown || !searchResults || searchResults.length === 0) return;

    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setHighlightedIndex(prev => 
          prev < searchResults.length - 1 ? prev + 1 : 0
        );
        break;
      case "ArrowUp":
        e.preventDefault();
        setHighlightedIndex(prev => 
          prev > 0 ? prev - 1 : searchResults.length - 1
        );
        break;
      case "Enter":
        e.preventDefault();
        if (highlightedIndex >= 0 && highlightedIndex < searchResults.length) {
          handleSelectItem(searchResults[highlightedIndex]);
        }
        break;
      case "Escape":
        setShowDropdown(false);
        setHighlightedIndex(-1);
        break;
    }
  }, [showDropdown, searchResults, highlightedIndex, handleSelectItem]);

  // Analyze
  const handleAnalyze = useCallback(async () => {
    if (!selectedItem) return;

    if (analyzing) {
      abortRef.current?.abort();
      return;
    }

    const controller = new AbortController();
    abortRef.current = controller;
    setAnalyzing(true);
    setProgress(t("scanStarting"));
    setResult(null);
    setIndustryCoverage(null);
    setIndustryCoverageMeta("");

    const params: IndustryParams = {
      type_id: selectedItem.type_id,
      runs,
      activity_mode: activityMode,
      me,
      te,
      system_name: systemName,
      station_id: selectedStationId > 0 ? selectedStationId : undefined,
      facility_tax: facilityTax,
      structure_bonus: structureBonus,
      broker_fee: brokerFee,
      sales_tax_percent: salesTaxPercent,
      max_depth: 10,
      own_blueprint: ownBlueprint,
      blueprint_cost: ownBlueprint ? 0 : blueprintCost,
      blueprint_is_bpo: blueprintIsBPO,
      invention_chance: activityMode === "invention" ? inventionChance : 0,
      decryptor_cost: activityMode === "invention" ? decryptorCost : 0,
      invention_output_runs: activityMode === "invention" ? inventionOutputRuns : 0,
    };

    try {
      const analysis = await analyzeIndustry(params, setProgress, controller.signal);
      setResult(analysis);
      setProgress("");
    } catch (e: unknown) {
      if (e instanceof Error && e.name === "AbortError") {
        setProgress("");
      } else if (e instanceof Error) {
        setProgress(t("errorPrefix") + e.message);
        onError?.(e.message);
      }
    } finally {
      setAnalyzing(false);
    }
  }, [analyzing, selectedItem, runs, activityMode, me, te, systemName, selectedStationId, facilityTax, structureBonus, brokerFee, salesTaxPercent, ownBlueprint, blueprintCost, blueprintIsBPO, inventionChance, decryptorCost, inventionOutputRuns, t, onError]);

  const clearPlanPreview = useCallback(() => {
    setLastLedgerPlanPreview(null);
    setLastLedgerPlanPreviewPatch(null);
  }, []);

  const operationsBoardsProps = useMemo(() => ({
    ctx: {
      jobsWorkspaceTab,
      ledgerSnapshot,
      rebalanceInventoryScope,
      setRebalanceInventoryScope,
      rebalanceLookbackDays,
      setRebalanceLookbackDays,
      rebalanceStrategy,
      setRebalanceStrategy,
      rebalanceWarehouseScope,
      setRebalanceWarehouseScope,
      blueprintSyncDefaultBPCRuns,
      setBlueprintSyncDefaultBPCRuns,
      syncingLedgerBlueprintPool,
      handleSyncLedgerBlueprintPoolFromAssets,
      rebalanceUseSelectedStation,
      setRebalanceUseSelectedStation,
      handleRebalanceLedgerMaterialsFromInventory,
      rebalancingLedgerMaterials,
      selectedLedgerTaskIDs,
      bulkLedgerTaskPriority,
      setBulkLedgerTaskPriority,
      handleBulkSetLedgerTaskPriority,
      updatingLedgerTasksBulk,
      handleBulkSetLedgerTaskStatus,
      setSelectedLedgerTaskIDs,
      allVisibleLedgerTasksSelected,
      handleSelectAllVisibleLedgerTasks,
      selectedLedgerTaskIDSet,
      toggleLedgerTaskSelection,
      industryTaskStatusClass,
      formatUtcShort,
      handleSetLedgerTaskPriority,
      updatingLedgerTaskId,
      handleSetLedgerTaskStatus,
      taskDependencyBoard,
    },
  }), [
    jobsWorkspaceTab,
    ledgerSnapshot,
    rebalanceInventoryScope,
    rebalanceLookbackDays,
    rebalanceStrategy,
    rebalanceWarehouseScope,
    blueprintSyncDefaultBPCRuns,
    syncingLedgerBlueprintPool,
    handleSyncLedgerBlueprintPoolFromAssets,
    rebalanceUseSelectedStation,
    handleRebalanceLedgerMaterialsFromInventory,
    rebalancingLedgerMaterials,
    selectedLedgerTaskIDs,
    bulkLedgerTaskPriority,
    handleBulkSetLedgerTaskPriority,
    updatingLedgerTasksBulk,
    handleBulkSetLedgerTaskStatus,
    allVisibleLedgerTasksSelected,
    handleSelectAllVisibleLedgerTasks,
    selectedLedgerTaskIDSet,
    toggleLedgerTaskSelection,
    handleSetLedgerTaskPriority,
    updatingLedgerTaskId,
    handleSetLedgerTaskStatus,
    taskDependencyBoard,
  ]);

  const plannerBuilderProps = useMemo(() => ({
    ctx: {
      jobsWorkspaceTab,
      useVisualPlanBuilder,
      planBuilderCompactMode,
      setPlanBuilderCompactMode,
      planBuilderPageSize,
      setPlanBuilderPageSize,
      togglePlanBuilderSection,
      planBuilderCollapsed,
      planDraftTasks,
      visualTaskBlueprintBindingStats,
      visiblePlanDraftTasks,
      taskPageStart,
      taskConstraintNumber,
      planTaskBlueprintOptionByPair,
      planTaskBlueprintOptionByType,
      planTaskBlueprintOptions,
      updateVisualTaskRow,
      removeVisualTaskRow,
      updateVisualTaskConstraints,
      updateVisualTaskConstraint,
      planBuilderPage,
      changePlanBuilderPage,
      planBuilderTotalPages,
      planDraftJobs,
      visiblePlanDraftJobs,
      jobPageStart,
      updateVisualJobRow,
      removeVisualJobRow,
      planDraftMaterials,
      visiblePlanDraftMaterials,
      materialPageStart,
      updateVisualMaterialRow,
      removeVisualMaterialRow,
      planDraftBlueprints,
      visiblePlanDraftBlueprints,
      blueprintPageStart,
      updateVisualBlueprintRow,
      removeVisualBlueprintRow,
    },
  }), [
    jobsWorkspaceTab,
    useVisualPlanBuilder,
    planBuilderCompactMode,
    planBuilderPageSize,
    togglePlanBuilderSection,
    planBuilderCollapsed,
    planDraftTasks,
    visualTaskBlueprintBindingStats,
    visiblePlanDraftTasks,
    taskPageStart,
    taskConstraintNumber,
    planTaskBlueprintOptionByPair,
    planTaskBlueprintOptionByType,
    planTaskBlueprintOptions,
    updateVisualTaskRow,
    removeVisualTaskRow,
    updateVisualTaskConstraints,
    updateVisualTaskConstraint,
    planBuilderPage,
    changePlanBuilderPage,
    planBuilderTotalPages,
    planDraftJobs,
    visiblePlanDraftJobs,
    jobPageStart,
    updateVisualJobRow,
    removeVisualJobRow,
    planDraftMaterials,
    visiblePlanDraftMaterials,
    materialPageStart,
    updateVisualMaterialRow,
    removeVisualMaterialRow,
    planDraftBlueprints,
    visiblePlanDraftBlueprints,
    blueprintPageStart,
    updateVisualBlueprintRow,
    removeVisualBlueprintRow,
  ]);

  const operationsJobsProps = useMemo(() => ({
    ctx: {
      jobsWorkspaceTab,
      ledgerData,
      formatISK,
      industryJobStatusClass,
      formatUtcShort,
      selectedLedgerJobIDs,
      updatingLedgerJobsBulk,
      handleBulkSetLedgerJobStatus,
      setSelectedLedgerJobIDs,
      allVisibleLedgerJobsSelected,
      handleSelectAllVisibleLedgerJobs,
      selectedLedgerJobIDSet,
      toggleLedgerJobSelection,
      handleSetLedgerJobStatus,
      updatingLedgerJobId,
    },
  }), [
    jobsWorkspaceTab,
    ledgerData,
    selectedLedgerJobIDs,
    updatingLedgerJobsBulk,
    handleBulkSetLedgerJobStatus,
    allVisibleLedgerJobsSelected,
    handleSelectAllVisibleLedgerJobs,
    selectedLedgerJobIDSet,
    toggleLedgerJobSelection,
    handleSetLedgerJobStatus,
    updatingLedgerJobId,
  ]);

  return (
    <div className={`flex-1 flex flex-col min-h-0 ${industryInnerTab === "jobs" ? "overflow-y-auto eve-scrollbar" : ""}`}>
      <div className="shrink-0 m-2 mb-0">
        <div className="inline-flex rounded-sm border border-eve-border overflow-hidden">
          <button
            type="button"
            onClick={() => setIndustryInnerTab("analysis")}
            className={`px-3 py-1.5 text-xs font-semibold uppercase tracking-wide transition-colors ${
              industryInnerTab === "analysis"
                ? "bg-eve-accent/20 text-eve-accent"
                : "bg-eve-panel text-eve-dim hover:text-eve-text"
            }`}
          >
            Analysis
          </button>
          <button
            type="button"
            onClick={() => setIndustryInnerTab("jobs")}
            className={`px-3 py-1.5 text-xs font-semibold uppercase tracking-wide transition-colors ${
              industryInnerTab === "jobs"
                ? "bg-eve-accent/20 text-eve-accent"
                : "bg-eve-panel text-eve-dim hover:text-eve-text"
            }`}
          >
            Jobs
          </button>
        </div>
      </div>

      {industryInnerTab === "analysis" && (
      <>
      {/* Settings Panel */}
      <div className="shrink-0 m-2">
        <TabSettingsPanel
          title={t("industrySettings")}
          hint={t("industrySettingsHint")}
          icon="🏭"
          defaultExpanded={true}
          persistKey="industry"
          help={{ stepKeys: ["helpIndustryStep1", "helpIndustryStep2", "helpIndustryStep3"], wikiSlug: "Industry-Chain-Optimizer" }}
        >
          {/* Item Search */}
          <div className="mb-4">
            <SettingsField label={t("industrySelectItem")}>
              <div className="relative">
                <input
                  ref={inputRef}
                  type="text"
                  value={searchQuery}
                  onChange={(e) => handleSearch(e.target.value)}
                  onFocus={() => searchResults?.length > 0 && setShowDropdown(true)}
                  onKeyDown={handleKeyDown}
                  placeholder={t("industrySearchPlaceholder")}
                  className="w-full px-3 py-1.5 bg-eve-input border border-eve-border rounded-sm text-eve-text text-sm
                           focus:outline-none focus:border-eve-accent focus:ring-1 focus:ring-eve-accent/30 transition-colors"
                  autoComplete="off"
                />
                {searching && (
                  <div className="absolute right-2 top-1/2 -translate-y-1/2">
                    <span className="w-4 h-4 border-2 border-eve-accent border-t-transparent rounded-full animate-spin inline-block" />
                  </div>
                )}
                {showDropdown && searchResults && searchResults.length > 0 && (
                  <div 
                    ref={dropdownRef}
                    className="absolute z-50 w-full mt-1 bg-eve-dark border border-eve-border rounded-sm shadow-lg max-h-60 overflow-auto"
                  >
                    {searchResults.map((item, index) => (
                      <button
                        key={item.type_id}
                        onClick={() => handleSelectItem(item)}
                        onMouseEnter={() => setHighlightedIndex(index)}
                        className={`w-full px-3 py-2 text-left text-sm transition-colors flex items-center justify-between ${
                          index === highlightedIndex
                            ? "bg-eve-accent/20 text-eve-accent"
                            : "text-eve-text hover:bg-eve-accent/10"
                        } ${!item.has_blueprint ? "opacity-60" : ""}`}
                      >
                        <span>
                          <HighlightMatch text={item.type_name} query={searchQuery} />
                        </span>
                        {item.has_blueprint ? (
                          <span className="text-[10px] px-1.5 py-0.5 bg-green-500/20 text-green-400 rounded-sm ml-2">BP</span>
                        ) : (
                          <span className="text-[10px] px-1.5 py-0.5 bg-eve-dim/20 text-eve-dim rounded-sm ml-2">No BP</span>
                        )}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            </SettingsField>
          </div>

          {/* Location settings (System, Station, Include Structures) */}
          <div className="mb-3">
            <SettingsGrid cols={3}>
              <SettingsField label={t("system")}>
                <SystemAutocomplete value={systemName} onChange={setSystemName} isLoggedIn={isLoggedIn} />
              </SettingsField>
              <SettingsField label={t("stationSelect")}>
                {loadingStations || loadingStructures ? (
                  <div className="h-[34px] flex items-center text-xs text-eve-dim">
                    {loadingStructures ? t("loadingStructures") : t("loadingStations")}
                  </div>
                ) : allStations.length === 0 ? (
                  <div className="h-[34px] flex items-center text-xs text-eve-dim">
                    {stations.length === 0 && !isLoggedIn
                      ? t("noNpcStationsLoginHint")
                      : stations.length === 0 && isLoggedIn && !includeStructures
                        ? t("noNpcStationsToggleHint")
                        : includeStructures
                          ? t("noStationsOrInaccessible")
                          : t("noStations")}
                  </div>
                ) : (
                  <SettingsSelect
                    value={selectedStationId}
                    onChange={(v) => setSelectedStationId(Number(v))}
                    options={[
                      { value: 0, label: t("allStations") },
                      ...allStations.map(st => ({
                        value: st.id,
                        label: st.is_structure ? `🏗️ ${st.name}` : st.name
                      }))
                    ]}
                  />
                )}
              </SettingsField>
              {isLoggedIn && (
                <SettingsField label={t("includeStructures")}>
                  <SettingsCheckbox
                    checked={includeStructures}
                    onChange={setIncludeStructures}
                  />
                </SettingsField>
              )}
            </SettingsGrid>
          </div>

          {/* Production parameters (Runs, ME, TE, Facility Tax) */}
          <div className="mb-3">
            <SettingsGrid cols={5}>
              <SettingsField label="Mode">
                <SettingsSelect
                  value={activityMode}
                  onChange={(value) => setActivityMode(value as IndustryActivityMode)}
                  options={[
                    { value: "auto", label: "Auto" },
                    { value: "manufacturing", label: "Manufacturing" },
                    { value: "reaction", label: "Reaction" },
                    { value: "invention", label: "Invention + build" },
                  ]}
                />
              </SettingsField>
              <SettingsField label={t("industryRuns")}>
                <SettingsNumberInput value={runs} onChange={setRuns} min={1} max={10000} />
              </SettingsField>
              <SettingsField label={t("industryME")}>
                <SettingsNumberInput value={me} onChange={setME} min={0} max={10} />
              </SettingsField>
              <SettingsField label={t("industryTE")}>
                <SettingsNumberInput value={te} onChange={setTE} min={0} max={20} />
              </SettingsField>
              <SettingsField label={t("industryFacilityTax")}>
                <SettingsNumberInput value={facilityTax} onChange={setFacilityTax} min={0} max={50} step={0.1} />
              </SettingsField>
            </SettingsGrid>
          </div>

          {activityMode === "invention" && (
            <div className="mt-3 pt-3 border-t border-eve-border/30">
              <div className="text-[10px] uppercase tracking-wider text-eve-dim mb-2">Invention</div>
              <SettingsGrid cols={3}>
                <SettingsField label="Chance override %">
                  <SettingsNumberInput value={inventionChance} onChange={setInventionChance} min={0} max={100} step={0.1} />
                </SettingsField>
                <SettingsField label="Decryptor / attempt">
                  <SettingsNumberInput value={decryptorCost} onChange={setDecryptorCost} min={0} max={100000000000} step={100000} />
                </SettingsField>
                <SettingsField label="BPC runs / success">
                  <SettingsNumberInput value={inventionOutputRuns} onChange={setInventionOutputRuns} min={0} max={100000} />
                </SettingsField>
              </SettingsGrid>
              <div className="text-[10px] text-eve-dim mt-1">
                Zero uses SDE probability and output runs.
              </div>
            </div>
          )}

          {/* After broker: broker fee and sales tax */}
          <div className="mt-3 pt-3 border-t border-eve-border/30">
            <div className="text-[10px] uppercase tracking-wider text-eve-dim mb-2">{t("industryAfterBroker")}</div>
            <SettingsGrid cols={5}>
              <SettingsField label={t("brokerFee")}>
                <SettingsNumberInput value={brokerFee} onChange={setBrokerFee} min={0} max={10} step={0.1} />
              </SettingsField>
              <SettingsField label={t("salesTax")}>
                <SettingsNumberInput value={salesTaxPercent} onChange={setSalesTaxPercent} min={0} max={100} step={0.1} />
              </SettingsField>
            </SettingsGrid>
          </div>

          {/* Advanced Options */}
          <details className="mt-3 group">
            <summary className="cursor-pointer text-xs text-eve-dim hover:text-eve-accent transition-colors flex items-center gap-1">
              <span className="group-open:rotate-90 transition-transform">▶</span>
              {t("advancedFilters")}
            </summary>
            <div className="mt-3 pt-3 border-t border-eve-border/30">
              <SettingsGrid cols={3}>
                <SettingsField label={t("industryStructureBonus")}>
                  <SettingsNumberInput value={structureBonus} onChange={setStructureBonus} min={0} max={5} step={0.1} />
                </SettingsField>
              </SettingsGrid>
            </div>
          </details>

          {/* Blueprint ownership */}
          <div className="mt-3 pt-3 border-t border-eve-border/30">
            <label className="flex items-center gap-2 cursor-pointer text-xs">
              <input
                type="checkbox"
                checked={ownBlueprint}
                onChange={(e) => setOwnBlueprint(e.target.checked)}
                className="accent-eve-accent"
              />
              <span className="text-eve-text">{t("industryOwnBlueprint")}</span>
            </label>
            {!ownBlueprint && (
              <div className="mt-2 flex items-center gap-3 flex-wrap">
                <div className="flex items-center gap-1.5">
                  <label className="text-eve-dim text-xs">{t("industryBlueprintCost")}</label>
                  <input
                    type="number"
                    min="0"
                    value={blueprintCost || ""}
                    onChange={(e) => setBlueprintCost(parseFloat(e.target.value) || 0)}
                    className="w-32 px-1.5 py-0.5 bg-eve-input border border-eve-border rounded-sm text-xs text-eve-text font-mono"
                  />
                </div>
                <div className="flex items-center gap-1.5">
                  <button
                    onClick={() => setBlueprintIsBPO(true)}
                    className={`px-2 py-0.5 text-[10px] font-semibold rounded-sm border transition-colors ${blueprintIsBPO ? "border-eve-accent bg-eve-accent/10 text-eve-accent" : "border-eve-border text-eve-dim hover:text-eve-text"}`}
                  >
                    {t("industryBPO")}
                  </button>
                  <button
                    onClick={() => setBlueprintIsBPO(false)}
                    className={`px-2 py-0.5 text-[10px] font-semibold rounded-sm border transition-colors ${!blueprintIsBPO ? "border-eve-accent bg-eve-accent/10 text-eve-accent" : "border-eve-border text-eve-dim hover:text-eve-text"}`}
                  >
                    {t("industryBPC")}
                  </button>
                </div>
                {blueprintIsBPO && blueprintCost > 0 && runs > 0 && (
                  <span className="text-[10px] text-eve-dim italic">
                    ≈ {formatISK(blueprintCost / runs)} / run
                  </span>
                )}
              </div>
            )}
          </div>

          {/* Analyze Button */}
          <div className="mt-4 pt-3 border-t border-eve-border/30 flex items-center gap-4 flex-wrap">
            <button
              onClick={handleAnalyze}
              disabled={!selectedItem || (selectedItem && !selectedItem.has_blueprint)}
              className={`px-5 py-1.5 rounded-sm text-xs font-semibold uppercase tracking-wider transition-all
                ${analyzing
                  ? "bg-eve-error/80 text-white hover:bg-eve-error"
                  : "bg-eve-accent text-eve-dark hover:bg-eve-accent-hover shadow-eve-glow"
                }
                disabled:bg-eve-input disabled:text-eve-dim disabled:cursor-not-allowed disabled:shadow-none`}
            >
              {analyzing ? t("stop") : t("industryAnalyze")}
            </button>
            {progress && <span className="text-xs text-eve-dim">{progress}</span>}
            {selectedItem && !selectedItem.has_blueprint && (
              <span className="text-xs text-yellow-400">
                {t("industryNoBlueprint")}
              </span>
            )}
          </div>
        </TabSettingsPanel>
      </div>
      </>
      )}

      {/* Industry Ledger Panel (M1 foundation) */}
      {industryInnerTab === "jobs" && (
        <Suspense fallback={<div className="m-2 text-xs text-eve-dim">Loading jobs workspace...</div>}>
          <IndustryJobsLedgerPanel
            isLoggedIn={isLoggedIn}
            ledgerProjectsLoading={ledgerProjectsLoading}
            jobsWorkspaceTab={jobsWorkspaceTab}
            setJobsWorkspaceTab={setJobsWorkspaceTab}
            warningsCount={plannerWarnings.length}
            missingBindings={visualTaskBlueprintBindingStats.missing}
            activeJobs={activeJobCount}
            projectHeaderProps={{
              newLedgerProjectName,
              setNewLedgerProjectName,
              newLedgerProjectStrategy,
              setNewLedgerProjectStrategy,
              creatingLedgerProject,
              handleCreateLedgerProject,
              refreshLedgerProjects,
              selectedLedgerProjectId,
              setSelectedLedgerProjectId,
              ledgerProjects,
              ledgerLoading,
              ledgerData,
              ledgerSnapshotLoading,
              ledgerSnapshot,
              handleLoadLedgerSnapshotToBuilder,
            }}
            guidePanelProps={{
              selectedProjectId: selectedLedgerProjectId,
              hasPlanSeedSource,
              hasVisualPlanRows,
              hasPreview: Boolean(lastLedgerPlanPreviewPatch),
              previewStale: isLastLedgerPreviewStale,
              strictBlueprintApplyBlocked,
              missingBindings: visualTaskBlueprintBindingStats.missing,
              previewing: previewingLedgerPlan,
              applying: applyingLedgerPlan,
              lastPreviewPatchExists: Boolean(lastLedgerPlanPreviewPatch),
              onGenerateDraft: handleGeneratePlanDraft,
              onPreview: () => { void handlePreviewCurrentAnalysisToLedgerPlan(); },
              onApplyPreview: () => { void handleApplyLastPreviewToLedgerPlan(); },
              onApplyCurrent: () => { void handleApplyCurrentAnalysisToLedgerPlan(); },
              onOpenPlanner: () => setJobsWorkspaceTab("planning"),
              onOpenOperations: () => setJobsWorkspaceTab("operations"),
            }}
            planningActionsProps={{
              jobsWorkspaceTab,
              handleGeneratePlanDraft,
              previewingLedgerPlan,
              selectedLedgerProjectId,
              hasVisualPlanRows,
              result,
              selectedItem,
              handlePreviewCurrentAnalysisToLedgerPlan,
              applyingLedgerPlan,
              lastLedgerPlanPreviewPatch,
              strictBlueprintApplyBlocked,
              isLastLedgerPreviewStale,
              handleApplyLastPreviewToLedgerPlan,
              handleApplyCurrentAnalysisToLedgerPlan,
              addVisualTaskRow,
              addVisualJobRow,
              addVisualMaterialRow,
              addVisualBlueprintRow,
              autoFixVisualTaskBlueprintBindings,
              planDraftTasksLength: planDraftTasks.length,
              planTaskBlueprintOptionsLength: planTaskBlueprintOptions.length,
              visualTaskBlueprintBindingStatsMissing: visualTaskBlueprintBindingStats.missing,
              visualTaskBlueprintBindingStatsFallback: visualTaskBlueprintBindingStats.fallback,
              clearVisualPlanBuilder,
              replaceLedgerPlanOnApply,
              setReplaceLedgerPlanOnApply,
              useVisualPlanBuilder,
              setUseVisualPlanBuilder,
              strictBlueprintBindingMode,
              setStrictBlueprintBindingMode,
              planDraftJobsLength: planDraftJobs.length,
              planDraftMaterialsLength: planDraftMaterials.length,
              planDraftBlueprintsLength: planDraftBlueprints.length,
              lastLedgerPlanSummary,
              lastLedgerPlanPreview,
            }}
            warningLogProps={{
              warnings: plannerWarnings,
              onClear: () => setPlannerWarnings([]),
              sourceLabel: plannerWarningSourceLabel,
            }}
            workspaceStatusBoardsProps={{
              jobsWorkspaceTab,
              taskStatusDone,
              taskStatusTotal,
              taskStatusDonePct,
              taskStatusBoard,
              jobStatusDone,
              jobStatusTotal,
              jobStatusDonePct,
              jobStatusBoard,
              ledgerSnapshot,
              materialCoverageTotals,
            }}
            dependencyBoardProps={{ board: taskDependencyBoard }}
            schedulerPanelProps={{
              jobsWorkspaceTab,
              enablePlanScheduler,
              setEnablePlanScheduler,
              schedulerSlotCount,
              setSchedulerSlotCount,
              schedulerMaxRunsPerJob,
              setSchedulerMaxRunsPerJob,
              schedulerMaxDurationHours,
              setSchedulerMaxDurationHours,
              schedulerQueueStatus,
              setSchedulerQueueStatus,
            }}
            planPreviewPanelProps={{
              jobsWorkspaceTab,
              lastLedgerPlanPreview,
              isLastLedgerPreviewStale,
              clearPlanPreview,
            }}
            operationsBoardsProps={operationsBoardsProps}
            plannerBuilderProps={plannerBuilderProps}
            operationsJobsProps={operationsJobsProps}
          />
        </Suspense>
      )}

      {/* Results */}
      {industryInnerTab === "analysis" && result && (
        <Suspense fallback={<div className="m-2 text-xs text-eve-dim">Loading analysis view...</div>}>
          <IndustryAnalysisResultsPanel
            result={result}
            viewMode={viewMode}
            setViewMode={setViewMode}
            salesTaxPercent={salesTaxPercent}
            brokerFee={brokerFee}
            onOpenExecutionPlan={setExecPlanMaterial}
            isLoggedIn={isLoggedIn}
            coverage={industryCoverage}
            coverageLoading={industryCoverageLoading}
            coverageMeta={industryCoverageMeta}
            coverageScope={rebalanceInventoryScope}
            onCoverageScopeChange={setRebalanceInventoryScope}
            coverageUseSelectedStation={rebalanceUseSelectedStation}
            onCoverageUseSelectedStationChange={setRebalanceUseSelectedStation}
            coverageStationLabel={selectedStationLabel}
            coverageDefaultBPCRuns={blueprintSyncDefaultBPCRuns}
            onCoverageDefaultBPCRunsChange={setBlueprintSyncDefaultBPCRuns}
            onRefreshCoverage={handleCheckCurrentIndustryCoverage}
            onSeedLedgerDraft={handleSeedCurrentIndustryPlanFromAnalysis}
          />
        </Suspense>
      )}

      {/* Empty State */}
      {industryInnerTab === "analysis" && !result && !analyzing && (
        <div className="flex-1 flex items-center justify-center min-h-[200px]">
          <EmptyState reason="no_item_selected" wikiSlug="Industry-Chain-Optimizer" />
        </div>
      )}

      <ExecutionPlannerPopup
        open={execPlanMaterial !== null}
        onClose={() => setExecPlanMaterial(null)}
        typeID={execPlanMaterial?.type_id ?? 0}
        typeName={execPlanMaterial?.type_name ?? ""}
        regionID={result?.region_id ?? 0}
        defaultQuantity={execPlanMaterial?.quantity ?? 100}
        isBuy={true}
        brokerFeePercent={brokerFee}
        salesTaxPercent={salesTaxPercent}
      />
    </div>
  );
}

