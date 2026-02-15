import { useState, useCallback } from "react";
import { openMarketInGame, setWaypointInGame, openContractInGame } from "./api";
import { useI18n } from "./i18n";
import { useGlobalToast } from "../components/Toast";
import type { ContextMenuItem } from "../components/ContextMenu";

interface ContextMenuState {
  x: number;
  y: number;
  visible: boolean;
}

interface UseEveContextMenuOptions {
  typeID?: number;
  typeName?: string;
  solarSystemID?: number;
  contractID?: number;
  isLoggedIn?: boolean;
}

export function useEveContextMenu() {
  const { t } = useI18n();
  const { addToast } = useGlobalToast();
  const [contextMenu, setContextMenu] = useState<ContextMenuState>({
    x: 0,
    y: 0,
    visible: false,
  });

  const closeContextMenu = useCallback(() => {
    setContextMenu((prev) => ({ ...prev, visible: false }));
  }, []);

  const openContextMenu = useCallback((e: React.MouseEvent, options: UseEveContextMenuOptions) => {
    e.preventDefault();
    e.stopPropagation();

    setContextMenu({
      x: e.clientX,
      y: e.clientY,
      visible: true,
    });

    // Store options for menu items
    (setContextMenu as any).options = options;
  }, []);

  const buildMenuItems = useCallback((options: UseEveContextMenuOptions): ContextMenuItem[] => {
    const items: ContextMenuItem[] = [];

    if (!options.isLoggedIn) {
      items.push({
        label: t("loginRequired"),
        icon: "⚠️",
        onClick: () => {},
        disabled: true,
      });
      return items;
    }

    // Open Market Window
    if (options.typeID) {
      items.push({
        label: t("openMarket"),
        icon: "📊",
        onClick: async () => {
          try {
            await openMarketInGame(options.typeID!);
            addToast(t("actionSuccess"), "success", 2000);
          } catch (err: any) {
            addToast(t("actionFailed").replace("{error}", err.message), "error", 3000);
          }
        },
      });
    }

    // Set Destination
    if (options.solarSystemID) {
      items.push({
        label: t("setDestination"),
        icon: "🎯",
        onClick: async () => {
          try {
            await setWaypointInGame(options.solarSystemID!);
            addToast(t("actionSuccess"), "success", 2000);
          } catch (err: any) {
            addToast(t("actionFailed").replace("{error}", err.message), "error", 3000);
          }
        },
      });
    }

    // Open Contract
    if (options.contractID) {
      items.push({
        label: t("openContract"),
        icon: "📜",
        onClick: async () => {
          try {
            await openContractInGame(options.contractID!);
            addToast(t("actionSuccess"), "success", 2000);
          } catch (err: any) {
            addToast(t("actionFailed").replace("{error}", err.message), "error", 3000);
          }
        },
      });
    }

    return items;
  }, [t, addToast]);

  return {
    contextMenu,
    openContextMenu,
    closeContextMenu,
    buildMenuItems,
  };
}
