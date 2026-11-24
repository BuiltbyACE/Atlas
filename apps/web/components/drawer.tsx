"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

import { Icon } from "@/components/icons";
import { cx } from "@/components/ui";

/**
 * A right-hand panel over a dimmed page (prototype `.ax-drawer`): a navy
 * header, a scrolling body, Escape and the scrim close it. It is a modal
 * dialog, so focus moves in on open and returns to the opener on close.
 *
 * Rendered through a portal: the sticky top bar uses `backdrop-filter`, which
 * would otherwise become the containing block for `position: fixed`.
 */
export function Drawer({
  open,
  onClose,
  title,
  description,
  eyebrow,
  children,
  wide = false,
}: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  description?: ReactNode;
  eyebrow?: ReactNode;
  children: ReactNode;
  wide?: boolean;
}) {
  const [mounted, setMounted] = useState(false);
  const panel = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);

  useEffect(() => {
    closeRef.current = onClose;
  }, [onClose]);

  useEffect(() => setMounted(true), []);

  useEffect(() => {
    if (!open) return;
    const opener = document.activeElement as HTMLElement | null;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    // Focus the first field if there is one, else the panel itself.
    const timer = window.setTimeout(() => {
      const target = panel.current?.querySelector<HTMLElement>(
        "[data-autofocus], input:not([type=hidden]), select, textarea",
      );
      (target ?? panel.current)?.focus();
    }, 30);

    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.stopPropagation();
        closeRef.current();
      }
    };
    document.addEventListener("keydown", onKey);

    return () => {
      window.clearTimeout(timer);
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = previousOverflow;
      opener?.focus?.();
    };
  }, [open]);

  if (!mounted || !open) return null;

  return createPortal(
    <div className="app-shell font-sans">
      <button
        type="button"
        aria-label="Close panel"
        onClick={onClose}
        className="fixed inset-0 z-40 animate-fade-in cursor-pointer bg-[rgba(0,20,48,.38)]"
      />
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-label={typeof title === "string" ? title : undefined}
        tabIndex={-1}
        className={cx(
          "fixed bottom-0 right-0 top-0 z-[41] flex w-full animate-drawer-in flex-col overflow-y-auto bg-white text-night shadow-drawer outline-none",
          wide ? "max-w-[560px]" : "max-w-[500px]",
        )}
      >
        <div className="on-navy flex items-start gap-3 bg-navy p-6 text-white">
          <div className="min-w-0">
            {eyebrow && <div className="mb-3">{eyebrow}</div>}
            <h2 className="text-[23px] font-semibold leading-tight text-white">{title}</h2>
            {description && (
              <div className="mt-1.5 text-sm text-navy-lead">{description}</div>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="ml-auto grid h-11 w-11 flex-none place-items-center rounded-control border border-white/25 text-white transition hover:bg-white/10"
          >
            <Icon name="close" />
          </button>
        </div>
        <div className="flex-1">{children}</div>
      </div>
    </div>,
    document.body,
  );
}

/** A titled block inside a drawer (prototype `.tb-sec`). */
export function DrawerSection({
  title,
  children,
  last = false,
  id,
}: {
  title?: ReactNode;
  children: ReactNode;
  last?: boolean;
  id?: string;
}) {
  return (
    <section id={id} className={cx("px-6 py-5", !last && "border-b border-line")}>
      {title && (
        <h3 className="mb-3 text-xs font-semibold uppercase tracking-[0.09em] text-muted">
          {title}
        </h3>
      )}
      {children}
    </section>
  );
}
