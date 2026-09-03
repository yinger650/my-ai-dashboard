import { servicePathLabel } from "../lib/service-brief";

export function ServicePathLine({ path }: { path?: string | null }) {
  const value = path?.trim();
  if (!value) return null;
  return (
    <p className="mt-0.5 break-all font-mono text-[11px] leading-relaxed text-slate-500">
      {servicePathLabel(value)} {value}
    </p>
  );
}
