import { SystemAutocomplete } from "./SystemAutocomplete";
import { useI18n } from "@/lib/i18n";
import type { ScanParams } from "@/lib/types";

interface Props {
  params: ScanParams;
  onChange: (params: ScanParams) => void;
}

export function ParametersPanel({ params, onChange }: Props) {
  const { t } = useI18n();
  const set = <K extends keyof ScanParams>(key: K, value: ScanParams[K]) => {
    onChange({ ...params, [key]: value });
  };

  return (
    <div className="bg-eve-panel border border-eve-border rounded-sm p-4">
      <div className="grid grid-cols-4 gap-x-4 gap-y-3 items-end">
        <Field label={t("system")}>
          <SystemAutocomplete
            value={params.system_name}
            onChange={(v) => set("system_name", v)}
          />
        </Field>

        <Field label={t("cargoCapacity")}>
          <NumberInput
            value={params.cargo_capacity}
            onChange={(v) => set("cargo_capacity", v)}
            min={1}
            max={1000000}
          />
        </Field>

        <Field label={t("buyRadius")}>
          <NumberInput
            value={params.buy_radius}
            onChange={(v) => set("buy_radius", v)}
            min={1}
            max={50}
          />
        </Field>

        <Field label={t("sellRadius")}>
          <NumberInput
            value={params.sell_radius}
            onChange={(v) => set("sell_radius", v)}
            min={1}
            max={50}
          />
        </Field>

        <Field label={t("minMargin")}>
          <NumberInput
            value={params.min_margin}
            onChange={(v) => set("min_margin", v)}
            min={0.1}
            max={1000}
            step={0.1}
          />
        </Field>

        <Field label={t("salesTax")}>
          <NumberInput
            value={params.sales_tax_percent}
            onChange={(v) => set("sales_tax_percent", v)}
            min={0}
            max={100}
            step={0.1}
          />
        </Field>
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <label className="text-[11px] uppercase tracking-wider text-eve-dim font-medium">
        {label}
      </label>
      {children}
    </div>
  );
}

function NumberInput({
  value,
  onChange,
  min,
  max,
  step = 1,
}: {
  value: number;
  onChange: (v: number) => void;
  min: number;
  max: number;
  step?: number;
}) {
  return (
    <input
      type="number"
      value={value}
      onChange={(e) => {
        const v = parseFloat(e.target.value);
        if (!isNaN(v) && v >= min && v <= max) onChange(v);
      }}
      min={min}
      max={max}
      step={step}
      className="w-full px-3 py-1.5 bg-eve-input border border-eve-border rounded-sm text-eve-text text-sm font-mono
                 focus:outline-none focus:border-eve-accent focus:ring-1 focus:ring-eve-accent/30
                 transition-colors
                 [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
    />
  );
}
