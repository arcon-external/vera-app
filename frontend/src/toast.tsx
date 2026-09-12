import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";

export type ToastType = "success" | "error" | "info";

export interface ToastMessage {
  id: number;
  type: ToastType;
  message: string;
}

interface ToastContextValue {
  pushToast: (type: ToastType, message: string) => void;
  dismissToast: (id: number) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const nextId = useRef(1);
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const dismissToast = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id));
  }, []);

  const pushToast = useCallback((type: ToastType, message: string) => {
    setToasts((current) => {
      if (current.some((toast) => toast.message === message)) return current;
      const next = [...current, { id: nextId.current++, type, message }];
      return next.slice(-4);
    });
  }, []);

  const value = useMemo(() => ({ pushToast, dismissToast }), [pushToast, dismissToast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div
        aria-label="Notifications"
        className="pointer-events-none fixed right-4 top-4 z-40 flex w-[min(360px,calc(100vw-2rem))] flex-col gap-2"
      >
        {toasts.map((toast) => (
          <ToastItem key={toast.id} toast={toast} onDismiss={dismissToast} />
        ))}
      </div>
    </ToastContext.Provider>
  );
}

function ToastItem({
  toast,
  onDismiss,
}: {
  toast: ToastMessage;
  onDismiss: (id: number) => void;
}) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const startedAt = useRef(Date.now());
  const remaining = useRef(5000);

  const startTimer = useCallback(() => {
    startedAt.current = Date.now();
    timer.current = setTimeout(() => onDismiss(toast.id), remaining.current);
  }, [onDismiss, toast.id]);

  const pauseTimer = useCallback(() => {
    if (timer.current === null) return;
    clearTimeout(timer.current);
    remaining.current = Math.max(0, remaining.current - (Date.now() - startedAt.current));
    timer.current = null;
  }, []);

  useEffect(() => {
    startTimer();
    return () => {
      if (timer.current !== null) clearTimeout(timer.current);
    };
  }, [startTimer]);

  const styles: Record<ToastType, string> = {
    success: "border-emerald-200 bg-emerald-50 text-emerald-900",
    error: "border-red-200 bg-red-50 text-red-900",
    info: "border-slate-200 bg-white text-slate-800",
  };

  return (
    <div
      role={toast.type === "error" ? "alert" : "status"}
      aria-live={toast.type === "error" ? "assertive" : "polite"}
      onMouseEnter={pauseTimer}
      onMouseLeave={() => {
        if (timer.current === null && remaining.current > 0) startTimer();
      }}
      className={`pointer-events-auto flex min-h-12 items-start gap-3 rounded-xl border px-4 py-3 text-[13px] shadow-[0_8px_24px_rgba(15,23,42,0.10)] ${styles[toast.type]}`}
      style={{ animation: "toastIn 180ms cubic-bezier(0.23, 1, 0.32, 1) forwards" }}
    >
      <span aria-hidden="true" className="mt-1 h-2 w-2 shrink-0 rounded-full bg-current opacity-60" />
      <span className="flex-1 leading-relaxed">{toast.message}</span>
      <button
        type="button"
        aria-label="Dismiss notification"
        onClick={() => onDismiss(toast.id)}
        className="-mr-2 -mt-1 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-current opacity-60 transition hover:bg-black/5 hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-current/30 active:scale-[0.96]"
      >
        <span aria-hidden="true" className="text-[18px] leading-none">×</span>
      </button>
    </div>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) throw new Error("useToast must be used within ToastProvider");
  return context;
}
