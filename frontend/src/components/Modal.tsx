import { useEffect, useRef, useId, useState } from "react";
import { createPortal } from "react-dom";

interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  width?: string;
}

export function Modal({ open, onClose, title, children, width = "max-w-4xl" }: ModalProps) {
  const overlayRef = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    if (open) {
      document.addEventListener("keydown", handleEsc);
      document.body.style.overflow = "hidden";
    }
    return () => {
      document.removeEventListener("keydown", handleEsc);
      document.body.style.overflow = "";
    };
  }, [open, onClose]);

  if (!open || !mounted) return null;

  return createPortal(
    <div
      ref={overlayRef}
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      className="fixed inset-0 z-50 flex items-end sm:items-center justify-center bg-black/70 backdrop-blur-sm"
      onClick={(e) => {
        if (e.target === overlayRef.current) onClose();
      }}
    >
      <div
        className={`w-full ${width} mx-2 sm:mx-4 h-[95vh] sm:h-auto sm:max-h-[85vh] flex flex-col bg-eve-dark border border-eve-border rounded-t-lg sm:rounded-sm shadow-2xl`}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-3 sm:px-4 py-2.5 sm:py-3 border-b border-eve-border bg-eve-panel shrink-0">
          <h2 id={titleId} className="text-xs sm:text-sm font-semibold uppercase tracking-wider text-eve-accent">
            {title}
          </h2>
          <button
            onClick={onClose}
            aria-label="Close dialog"
            className="text-eve-dim hover:text-eve-text transition-colors text-lg leading-none p-1"
          >
            &#10005;
          </button>
        </div>
        {/* Content */}
        <div className="flex-1 min-h-0 overflow-auto">{children}</div>
      </div>
    </div>,
    document.body,
  );
}
