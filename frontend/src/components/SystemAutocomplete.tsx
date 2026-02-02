import { useEffect, useRef, useState } from "react";
import { autocomplete } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

interface Props {
  value: string;
  onChange: (value: string) => void;
}

export function SystemAutocomplete({ value, onChange }: Props) {
  const { t } = useI18n();
  const [query, setQuery] = useState(value);
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [open, setOpen] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setQuery(value);
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

  return (
    <div ref={containerRef} className="relative">
      <input
        type="text"
        value={query}
        onChange={(e) => handleInput(e.target.value)}
        onKeyDown={handleKeyDown}
        onFocus={() => suggestions.length > 0 && setOpen(true)}
        placeholder={t("systemPlaceholder")}
        className="w-full px-3 py-1.5 bg-eve-input border border-eve-border rounded-sm text-eve-text
                   placeholder:text-eve-dim text-sm font-mono
                   focus:outline-none focus:border-eve-accent focus:ring-1 focus:ring-eve-accent/30
                   transition-colors"
      />
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
    </div>
  );
}
