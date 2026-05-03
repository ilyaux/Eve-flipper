import { useMemo } from "react";
import { type TranslationKey } from "../../lib/i18n";
import type { CharacterIndustryJob } from "../../lib/types";
import { StatCard } from "./shared";

interface IndustryJobsTabProps {
  jobs: CharacterIndustryJob[];
  formatIsk: (v: number) => string;
  formatDate: (d: string) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

export function IndustryJobsTab({ jobs, formatIsk, formatDate, t }: IndustryJobsTabProps) {
  const summary = useMemo(() => summarizeJobs(jobs), [jobs]);

  if (jobs.length === 0) {
    return (
      <div className="border border-eve-border rounded-sm bg-eve-panel/40 p-6 text-center">
        <div className="text-sm text-eve-text">{t("industryJobsNoLive")}</div>
        <div className="mt-1 text-xs text-eve-dim">{t("industryJobsNoLiveHint")}</div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 lg:grid-cols-5 gap-3">
        <StatCard label={t("industryJobsActive")} value={String(jobs.length)} />
        <StatCard label={t("industryJobsManufacturing")} value={String(summary.manufacturing)} />
        <StatCard label={t("industryJobsResearch")} value={String(summary.research)} />
        <StatCard label={t("industryJobsCost")} value={`${formatIsk(summary.cost)} ISK`} color="text-eve-warning" />
        <StatCard label={t("industryJobsNextDone")} value={summary.nextEnd ? formatDate(summary.nextEnd) : "-"} color="text-eve-accent" />
      </div>

      <section className="border border-eve-border rounded-sm overflow-hidden">
        <table className="w-full text-xs">
          <thead className="bg-eve-panel">
            <tr className="text-eve-dim">
              <th className="px-3 py-2 text-left">{t("industryJobsActivity")}</th>
              <th className="px-3 py-2 text-left">{t("colItemName")}</th>
              <th className="px-3 py-2 text-left">{t("industryJobsFacility")}</th>
              <th className="px-3 py-2 text-right">{t("charQty")}</th>
              <th className="px-3 py-2 text-right">{t("industryJobsCost")}</th>
              <th className="px-3 py-2 text-left">{t("industryJobsProgress")}</th>
              <th className="px-3 py-2 text-left">{t("industryJobsEnd")}</th>
            </tr>
          </thead>
          <tbody>
            {jobs.map((job) => {
              const progress = jobProgress(job);
              const itemName = job.product_type_name || job.blueprint_type_name || `Type #${job.product_type_id || job.blueprint_type_id}`;
              return (
                <tr key={job.job_id} className="border-t border-eve-border/50 hover:bg-eve-panel/50">
                  <td className="px-3 py-2">
                    <span className="inline-flex px-1.5 py-0.5 rounded-sm border border-eve-border bg-eve-dark text-[10px] uppercase tracking-wide text-eve-dim">
                      {activityLabel(job.activity_id)}
                    </span>
                  </td>
                  <td className="px-3 py-2 text-eve-text max-w-[260px] truncate" title={itemName}>{itemName}</td>
                  <td className="px-3 py-2 text-eve-dim max-w-[220px] truncate" title={job.facility_name}>{job.facility_name || `#${job.facility_id}`}</td>
                  <td className="px-3 py-2 text-right font-mono text-eve-text">{job.runs.toLocaleString()}</td>
                  <td className="px-3 py-2 text-right font-mono text-eve-warning">{formatIsk(job.cost)}</td>
                  <td className="px-3 py-2 min-w-[140px]">
                    <div className="h-1.5 bg-eve-dark border border-eve-border/60 rounded-sm overflow-hidden">
                      <div className="h-full bg-eve-accent" style={{ width: `${progress}%` }} />
                    </div>
                    <div className="mt-1 text-[10px] text-eve-dim">{progress.toFixed(0)}% / {job.status}</div>
                  </td>
                  <td className="px-3 py-2 text-eve-dim text-[11px]">{formatDate(job.end_date)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </section>
    </div>
  );
}

function summarizeJobs(jobs: CharacterIndustryJob[]) {
  let cost = 0;
  let manufacturing = 0;
  let research = 0;
  let nextEnd = "";
  for (const job of jobs) {
    cost += job.cost || 0;
    if (job.activity_id === 1 || job.activity_id === 11) manufacturing += 1;
    else research += 1;
    if (!nextEnd || job.end_date < nextEnd) nextEnd = job.end_date;
  }
  return { cost, manufacturing, research, nextEnd };
}

function jobProgress(job: CharacterIndustryJob): number {
  const start = new Date(job.start_date).getTime();
  const end = new Date(job.end_date).getTime();
  const now = Date.now();
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return 0;
  return Math.max(0, Math.min(100, ((now - start) / (end - start)) * 100));
}

function activityLabel(activityID: number): string {
  switch (activityID) {
    case 1:
      return "Manufacturing";
    case 3:
      return "TE";
    case 4:
      return "ME";
    case 5:
      return "Copy";
    case 8:
      return "Invention";
    case 9:
      return "Reaction";
    case 11:
      return "Reaction";
    default:
      return `#${activityID}`;
  }
}
