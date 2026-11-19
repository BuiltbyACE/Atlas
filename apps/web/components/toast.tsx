"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { Icon } from "@/components/icons";

type ToastTone = "success" | "error";

interface ToastState {
  id: number;
  message: string;
  tone: ToastTone;
}

type Notify = (message: string, tone?: ToastTone) => void;

const ToastContext = createContext<Notify>(() => undefined);

/**
 * One transient confirmation at a time, bottom-centre (prototype `.ax-toast`).
 * It only echoes what Go already confirmed or refused; nothing is decided here.
 * The live region is always mounted so screen readers announce each message.
 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<ToastState | null>(null);
  const timer = useRef<number | undefined>(undefined);

  const notify = useCallback<Notify>((message, tone = "success") => {
    window.clearTimeout(timer.current);
    setToast({ id: Date.now(), message, tone });
    timer.current = window.setTimeout(
      () => setToast(null),
      tone === "error" ? 6000 : 2800,
    );
  }, []);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  return (
    <ToastContext.Provider value={notify}>
      {children}
      <div aria-live="polite" role="status" className="pointer-events-none">
        {toast && (
          <div
            key={toast.id}
            className="pointer-events-auto fixed bottom-7 left-1/2 z-[60] flex max-w-[calc(100%-32px)] -translate-x-1/2 animate-toast-in items-center gap-2.5 rounded-[14px] bg-navy px-[18px] py-3 text-[14.5px] font-medium text-white shadow-pop"
          >
            <Icon
              name={toast.tone === "error" ? "alert" : "check"}
              className={toast.tone === "error" ? "text-[#FFB3A5]" : "text-[#7EE2A8]"}
            />
            <span>{toast.message}</span>
          </div>
        )}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): Notify {
  return useContext(ToastContext);
}
