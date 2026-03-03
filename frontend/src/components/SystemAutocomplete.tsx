import { useEffect, useRef, useState, type ReactNode } from "react";
import { autocomplete, getCharacterLocation, getStations } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

interface Props {
  value: string;
  onChange: (value: string) => void;
  /** If true (default) and user is logged in, shows a location button */
  showLocationButton?: boolean;
  /** Whether the user is logged in (enables location button) */
  isLoggedIn?: boolean;
  /** Whether "include structures" toggle is on */
  includeStructures?: boolean;
  /** Callback when structure toggle changes */
  onIncludeStructuresChange?: (v: boolean) => void;
  /** Optional extra action button rendered with right-side icons */
  extraAction?: ReactNode;
  /** Number of icon slots occupied by extraAction (for input right padding) */
  extraActionSlots?: number;
}

export function SystemAutocomplete({
  value,
  onChange,
  showLocationButton = true,
  isLoggedIn = false,
  includeStructures,
  onIncludeStructuresChange,
  extraAction,
  extraActionSlots = 0,
}: Props) {
  const { t } = useI18n();
  const [query, setQuery] = useState(value);
  const [locationLoading, setLocationLoading] = useState(false);
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [open, setOpen] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [noStations, setNoStations] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setQuery(value);
  }, [value]);

  // Check if selected system has NPC stations
  useEffect(() => {
    const systemName = value.trim();
    if (!systemName || systemName.length < 2) {
      setNoStations(false);
      return;
    }
    let cancelled = false;

    const run = async () => {
      try {
        const resp = await getStations(systemName);
        if (cancelled) return;

        const firstCheckNoStations = resp.system_id > 0 && resp.stations.length === 0;
        if (!firstCheckNoStations) {
          setNoStations(false);
          return;
        }

        // Rare backend race can return transient empty list for systems that have NPC stations.
        // Retry once before showing the warning.
        const retryResp = await getStations(systemName);
        if (cancelled) return;
        setNoStations(retryResp.system_id > 0 && retryResp.stations.length === 0);
      } catch {
        if (!cancelled) setNoStations(false);
      }
    };

    void run();
    return () => { cancelled = true; };
  }, [value]);

  // Cleanup autocomplete timer on unmount
  useEffect(() => {
    return () => clearTimeout(timerRef.current);
  }, []);

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const handleInput = (val: string) => {
    setQuery(val);
    clearTimeout(timerRef.current);
    if (val.length < 2) {
      setSuggestions([]);
      setOpen(false);
      return;
    }
    timerRef.current = setTimeout(async () => {
      const results = await autocomplete(val);
      setSuggestions(results);
      setSelectedIndex(0);
      setOpen(results.length > 0);
    }, 200);
  };

  const select = (name: string) => {
    setQuery(name);
    onChange(name);
    setOpen(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!open) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSelectedIndex((i) => Math.min(i + 1, suggestions.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSelectedIndex((i) => Math.max(i - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (suggestions[selectedIndex]) select(suggestions[selectedIndex]);
    } else if (e.key === "Escape") {
      setOpen(false);
    }
  };

  const handleLocationClick = async () => {
    setLocationLoading(true);
    try {
      const loc = await getCharacterLocation();
      if (loc?.solar_system_name) {
        setQuery(loc.solar_system_name);
        onChange(loc.solar_system_name);
      }
    } catch (err) {
      console.error("Failed to fetch location:", err);
    } finally {
      setLocationLoading(false);
    }
  };

  const showLocationBtn = showLocationButton && isLoggedIn;
  const showStructureBtn = isLoggedIn && onIncludeStructuresChange != null;
  const btnCount =
    (showLocationBtn ? 1 : 0) + (showStructureBtn ? 1 : 0) + extraActionSlots;

  return (
    <div ref={containerRef} className="relative">
      <input
        type="text"
        value={query}
        onChange={(e) => handleInput(e.target.value)}
        onKeyDown={handleKeyDown}
        onFocus={() => suggestions.length > 0 && setOpen(true)}
        placeholder={t("systemPlaceholder")}
        className={`w-full px-3 py-1.5 bg-eve-input border border-eve-border rounded-sm text-eve-text
                   placeholder:text-eve-dim text-sm font-mono
                   focus:outline-none focus:border-eve-accent focus:ring-1 focus:ring-eve-accent/30
                   transition-colors`}
        style={{ paddingRight: btnCount > 0 ? `${btnCount * 24 + 4}px` : undefined }}
      />
      <div className="absolute right-1 top-1/2 -translate-y-1/2 flex items-center gap-0.5">
        {extraAction}
        {showStructureBtn && (
          <button
            type="button"
            onClick={() => onIncludeStructuresChange!(!includeStructures)}
            title={includeStructures ? t("includeStructures") + " (ON)" : t("includeStructures") + " (OFF)"}
            className={`p-1 transition-colors ${
              includeStructures
                ? "text-eve-accent"
                : "text-eve-dim hover:text-eve-accent opacity-50 hover:opacity-100"
            }`}
          >
            <svg className="w-4 h-4" viewBox="0 0 24 24" fill={includeStructures ? "currentColor" : "none"} stroke="currentColor" strokeWidth="1.5">
              <path d="M12 2L3 9v12h18V9L12 2z" />
              <rect x="9" y="14" width="6" height="7" />
            </svg>
          </button>
        )}
        {showLocationBtn && (
          <button
            type="button"
            onClick={handleLocationClick}
            disabled={locationLoading}
            title={t("useCurrentLocation")}
            className="p-1 text-eve-dim hover:text-eve-accent
                       disabled:opacity-50 disabled:cursor-not-allowed
                       transition-colors"
          >
            {locationLoading ? (
              <svg className="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none">
                <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="2" strokeDasharray="32" strokeLinecap="round" />
              </svg>
            ) : (
              <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <circle cx="12" cy="12" r="3" />
                <path d="M12 2v4m0 12v4M2 12h4m12 0h4" />
              </svg>
            )}
          </button>
        )}
      </div>
      {open && suggestions.length > 0 && (
        <div className="absolute z-50 top-full left-0 right-0 mt-1 bg-eve-panel border border-eve-border rounded-sm shadow-eve-glow max-h-48 overflow-y-auto">
          {suggestions.map((name, i) => (
            <div
              key={name}
              onClick={() => select(name)}
              className={`px-3 py-1.5 text-sm cursor-pointer transition-colors ${
                i === selectedIndex
                  ? "bg-eve-accent/20 text-eve-accent"
                  : "text-eve-text hover:bg-eve-panel-hover"
              }`}
            >
              {name}
            </div>
          ))}
        </div>
      )}
      {noStations && !open && (
        <div className="absolute z-40 left-0 right-0 top-full mt-1 px-1 py-0.5 text-[10px] text-amber-400/80 leading-tight bg-eve-panel/95 border border-eve-border/50 rounded-sm">
          {!isLoggedIn
            ? t("noNpcStationsLoginHint")
            : !includeStructures
              ? t("noNpcStationsToggleHint")
              : null}
        </div>
      )}
    </div>
  );
}
