import { useEffect, useMemo, useState } from "react";
import { getCorpMembers } from "../../lib/api";
import { type TranslationKey } from "../../lib/i18n";
import type { CorpDashboard, CorpMember } from "../../lib/types";
import { KpiCard, TopContributorsTable } from "./shared";
export function MembersSection({
  dashboard,
  mode,
  formatIsk,
  t,
}: {
  dashboard: CorpDashboard;
  mode: "demo" | "live";
  formatIsk: (v: number) => string;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}) {
  const ms = dashboard.member_summary;
  const [members, setMembers] = useState<CorpMember[]>([]);
  const [membersLoading, setMembersLoading] = useState(false);
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<"name" | "last_login" | "system" | "ship">("last_login");
  const [sortAsc, setSortAsc] = useState(false);

  // Load members on first render
  useEffect(() => {
    setMembersLoading(true);
    getCorpMembers(mode)
      .then(setMembers)
      .catch(() => setMembers([]))
      .finally(() => setMembersLoading(false));
  }, [mode]);

  const toggleSort = (key: typeof sortKey) => {
    if (sortKey === key) setSortAsc(!sortAsc);
    else { setSortKey(key); setSortAsc(key === "name"); }
  };

  const filteredMembers = useMemo(() => {
    let list = [...members];
    if (search.trim()) {
      const q = search.toLowerCase();
      list = list.filter((m) => m.name.toLowerCase().includes(q) || m.system_name?.toLowerCase().includes(q) || m.ship_name?.toLowerCase().includes(q));
    }
    list.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "name": cmp = a.name.localeCompare(b.name); break;
        case "last_login": cmp = (a.last_login || "").localeCompare(b.last_login || ""); break;
        case "system": cmp = (a.system_name || "").localeCompare(b.system_name || ""); break;
        case "ship": cmp = (a.ship_name || "").localeCompare(b.ship_name || ""); break;
      }
      return sortAsc ? cmp : -cmp;
    });
    return list;
  }, [members, search, sortKey, sortAsc]);

  const categories = [
    { label: t("corpMiners"), value: ms.miners, color: "bg-blue-400" },
    { label: t("corpRatters"), value: ms.ratters, color: "bg-emerald-400" },
    { label: t("corpTraders"), value: ms.traders, color: "bg-amber-400" },
    { label: t("corpIndustrialists"), value: ms.industrialists, color: "bg-purple-400" },
    { label: t("corpPvPers"), value: ms.pvpers, color: "bg-red-400" },
    { label: t("corpOther"), value: ms.other, color: "bg-gray-400" },
  ];
  const total = ms.total_members || 1;

  const isOnline = (m: CorpMember) => {
    if (!m.last_login) return false;
    const diff = Date.now() - new Date(m.last_login).getTime();
    return diff < 15 * 60 * 1000;
  };

  const timeSince = (dateStr: string) => {
    if (!dateStr) return "Never";
    const diff = Date.now() - new Date(dateStr).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return "Just now";
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days}d ago`;
    return `${Math.floor(days / 30)}mo ago`;
  };

  const SortHeader = ({ label, field }: { label: string; field: typeof sortKey }) => (
    <th
      className="px-2 py-1.5 text-left cursor-pointer hover:text-eve-accent transition-colors select-none"
      onClick={() => toggleSort(field)}
    >
      {label} {sortKey === field && (sortAsc ? "↑" : "↓")}
    </th>
  );

  return (
    <div className="space-y-6">
      {/* Activity summary */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <KpiCard label={t("corpMembers")} value={String(ms.total_members)} />
        <KpiCard label={t("corpMembersActive7d")} value={String(ms.active_last_7d)} color="text-emerald-400" />
        <KpiCard label={t("corpMembersActive30d")} value={String(ms.active_last_30d)} color="text-eve-accent" />
        <KpiCard label={t("corpMembersInactive")} value={String(ms.inactive_30d)} color="text-eve-error" />
      </div>

      {/* Category breakdown bar */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpMemberBreakdown")}</div>
        <div className="flex h-6 rounded-sm overflow-hidden">
          {categories.filter((c) => c.value > 0).map((c, i) => (
            <div
              key={i}
              className={`${c.color} flex items-center justify-center text-[9px] font-bold text-black/70`}
              style={{ width: `${(c.value / total) * 100}%` }}
              title={`${c.label}: ${c.value}`}
            >
              {(c.value / total) * 100 > 8 ? c.value : ""}
            </div>
          ))}
        </div>
        <div className="flex flex-wrap gap-3 mt-3">
          {categories.filter((c) => c.value > 0).map((c, i) => (
            <div key={i} className="flex items-center gap-1.5 text-xs text-eve-dim">
              <div className={`w-2.5 h-2.5 rounded-sm ${c.color}`} />
              {c.label} ({c.value})
            </div>
          ))}
        </div>
      </div>

      {/* Top Contributors */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="text-[10px] text-eve-dim uppercase tracking-wider mb-3">{t("corpTopContributors")}</div>
        <TopContributorsTable contributors={dashboard.top_contributors} formatIsk={formatIsk} />
      </div>

      {/* Full member list */}
      <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
        <div className="flex items-center justify-between mb-3">
          <div className="text-[10px] text-eve-dim uppercase tracking-wider">{t("corpMembers")} ({members.length})</div>
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search..."
            className="px-2 py-1 text-xs bg-eve-dark border border-eve-border rounded-sm text-eve-text placeholder:text-eve-dim/50 w-48 focus:border-eve-accent outline-none"
          />
        </div>
        {membersLoading ? (
          <div className="flex items-center justify-center py-8 text-eve-dim text-xs">
            <span className="inline-block w-4 h-4 border-2 border-eve-accent/40 border-t-eve-accent rounded-full animate-spin mr-2" />
            Loading members...
          </div>
        ) : (
          <div className="border border-eve-border rounded-sm overflow-hidden max-h-[500px] overflow-y-auto">
            <table className="w-full text-xs">
              <thead className="bg-eve-panel sticky top-0 text-eve-dim">
                <tr>
                  <th className="px-2 py-1.5 w-6"></th>
                  <SortHeader label="Name" field="name" />
                  <SortHeader label="Last Seen" field="last_login" />
                  <SortHeader label="Ship" field="ship" />
                  <SortHeader label="System" field="system" />
                </tr>
              </thead>
              <tbody>
                {filteredMembers.map((m) => {
                  const online = isOnline(m);
                  return (
                    <tr key={m.character_id} className="border-t border-eve-border/30 hover:bg-eve-panel/50">
                      <td className="px-2 py-1.5 text-center">
                        <span className={`inline-block w-2 h-2 rounded-full ${online ? "bg-emerald-400" : "bg-eve-dim/30"}`} />
                      </td>
                      <td className="px-2 py-1.5">
                        <div className="flex items-center gap-2">
                          <img
                            src={`https://images.evetech.net/characters/${m.character_id}/portrait?size=32`}
                            alt=""
                            className="w-5 h-5 rounded-sm"
                          />
                          <span className="text-eve-text font-medium">{m.name}</span>
                        </div>
                      </td>
                      <td className={`px-2 py-1.5 ${online ? "text-emerald-400" : "text-eve-dim"}`}>
                        {timeSince(m.last_login)}
                      </td>
                      <td className="px-2 py-1.5 text-eve-dim max-w-[140px] truncate" title={m.ship_name}>
                        {m.ship_name || "—"}
                      </td>
                      <td className="px-2 py-1.5 text-eve-dim max-w-[120px] truncate" title={m.system_name}>
                        {m.system_name || "—"}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
