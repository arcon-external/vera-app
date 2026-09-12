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
        className="pointer-events-none fixed bottom-20 right-5 z-40 flex w-[min(360px,calc(100vw-2rem))] flex-col gap-2"
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
    success: "border-success-200 bg-success-50 text-success-900",
    error: "border-danger-200 bg-danger-50 text-danger-800",
    info: "border-rule bg-white text-ink-800",
  };

  const dots: Record<ToastType, string> = {
    success: "bg-success-500",
    error: "bg-danger-500",
    info: "bg-accent-500",
  };

  return (
    <div
      role={toast.type === "error" ? "alert" : "status"}
      aria-live={toast.type === "error" ? "assertive" : "polite"}
      onMouseEnter={pauseTimer}
      onMouseLeave={() => {
        if (timer.current === null && remaining.current > 0) startTimer();
      }}
      className={`pointer-events-auto flex min-h-11 animate-toast-in items-start gap-3 rounded-lg border px-4 py-3 text-small shadow-pop ${styles[toast.type]}`}
    >
      <span
        aria-hidden="true"
        className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${dots[toast.type]}`}
      />
      <span className="flex-1 leading-relaxed">{toast.message}</span>
      <button
        type="button"
        aria-label="Dismiss notification"
        onClick={() => onDismiss(toast.id)}
        className="-mr-1.5 -mt-1 flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-current opacity-55 transition-colors duration-150 hover:bg-black/5 hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-current/30 active:scale-[0.96]"
      >
        <span aria-hidden="true" className="text-base leading-none">
          ×
        </span>
      </button>
    </div>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) throw new Error("useToast must be used within ToastProvider");
  return context;
}
