import { useState } from "react";
import { ChevronDown, ChevronUp } from "lucide-react";
import { Markdown } from "./Markdown";
import { cn } from "../lib/utils";

export function CollapsibleText({
  text,
  maxChars = 220,
  className,
}: {
  text: string;
  maxChars?: number;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  const long = text.length > maxChars || text.split("\n").length > 5;

  return (
    <div className={cn("text-sm", className)}>
      <div className={cn(!open && long && "line-clamp-4")}>
        <Markdown>{text}</Markdown>
      </div>
      {long && (
        <button
          type="button"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            setOpen((v) => !v);
          }}
          className="mt-1 inline-flex items-center gap-0.5 text-xs text-indigo-400 hover:text-indigo-300"
        >
          {open ? (
            <>
              收起 <ChevronUp className="h-3 w-3" />
            </>
          ) : (
            <>
              展开 <ChevronDown className="h-3 w-3" />
            </>
          )}
        </button>
      )}
    </div>
  );
}
