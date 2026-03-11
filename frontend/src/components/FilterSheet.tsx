import type { ReactNode } from "react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { ScrollArea } from "@/components/ui/scroll-area";

interface FilterSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
}

export function FilterSheet({
  open,
  onOpenChange,
  title,
  description,
  children,
}: FilterSheetProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-[400px] sm:max-w-[400px]">
        <SheetHeader className="pr-6">
          <SheetTitle>{title}</SheetTitle>
          {description ? <SheetDescription>{description}</SheetDescription> : null}
        </SheetHeader>
        <ScrollArea className="mt-4 h-[calc(100vh-6rem)] pr-4">
          {children}
        </ScrollArea>
      </SheetContent>
    </Sheet>
  );
}
