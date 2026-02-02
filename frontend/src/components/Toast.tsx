import { useEffect, useState } from "react";

export interface ToastMessage {
  id: number;
  text: string;
}

let toastId = 0;

export function useToast() {
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const addToast = (text: string) => {
    const id = ++toastId;
    setToasts((prev) => [...prev, { id, text }]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 5000);
  };

  return { toasts, addToast };
}

export function ToastContainer({ toasts }: { toasts: ToastMessage[] }) {
  if (toasts.length === 0) return null;

  return (
    <div className="fixed top-4 right-4 z-[100] flex flex-col gap-2">
      {toasts.map((toast) => (
        <ToastItem key={toast.id} text={toast.text} />
      ))}
    </div>
  );
}

function ToastItem({ text }: { text: string }) {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    requestAnimationFrame(() => setVisible(true));
  }, []);

  return (
    <div
      className={`px-4 py-3 bg-eve-panel border border-eve-accent/50 rounded-sm shadow-eve-glow text-xs text-eve-text
        transition-all duration-300 ${visible ? "opacity-100 translate-x-0" : "opacity-0 translate-x-4"}`}
    >
      {text}
    </div>
  );
}
