import { createContext, useContext, useState, useCallback, type ReactNode } from "react";

export type Locale = "ru" | "en";

const translations = {
  ru: {
    // Header
    appTitle: "EVE Flipper",

    // Status
    sdeLoading: "SDE: загрузка...",
    sdeSystems: "систем",
    sdeTypes: "типов",
    esiApi: "ESI API",
    esiUnavailable: "ESI API: недоступен",

    // Parameters
    system: "Система",
    systemPlaceholder: "Система...",
    cargoCapacity: "Грузоподъёмность (m³)",
    buyRadius: "Радиус покупки (прыжки)",
    sellRadius: "Радиус продажи (прыжки)",
    minMargin: "Мин. маржа (%)",
    salesTax: "Налог продажи (%)",

    // Tabs
    tabRadius: "Флипер (радиус)",
    tabRegion: "Региональный арбитраж",
    tabContracts: "Арбитраж контрактов",

    // Buttons
    scan: "Сканировать",
    stop: "Остановить",

    // Table
    colItem: "Предмет",
    colBuyPrice: "Покупка ISK",
    colBuyStation: "Станция покупки",
    colSellPrice: "Продажа ISK",
    colSellStation: "Станция продажи",
    colMargin: "Маржа %",
    colUnitsToBuy: "Покупать",
    colAcceptQty: "Приём",
    colProfit: "Прибыль ISK",
    colProfitPerJump: "ISK/прыжок",
    colJumps: "Прыжки",

    // Contract table
    colTitle: "Название",
    colContractPrice: "Цена контракта",
    colMarketValue: "Рыночная стоимость",
    colContractProfit: "Прибыль",
    colContractMargin: "Маржа %",
    colVolume: "Объём m³",
    colStation: "Станция",
    colItems: "Предметов",
    colContractJumps: "Прыжки",
    colContractPPJ: "ISK/прыжок",
    foundContracts: "Найдено {count} контрактов",
    scanContractsPrompt: "Нажмите «Сканировать» для поиска контрактов",

    // Route finder
    tabRoute: "Маршрут",
    routeMinHops: "Мин. хопов",
    routeMaxHops: "Макс. хопов",
    routeFind: "Найти маршруты",
    routeFound: "Найдено {count} маршрутов",
    routePrompt: "Задайте параметры и нажмите «Найти маршруты»",
    routeColumn: "Маршрут",
    routeHopsCol: "Хопов",
    routeDetails: "Детали маршрута",
    routeTotalProfit: "Общая прибыль",
    routeTotalJumps: "Прыжков",
    routeJumpsUnit: "прыжков",
    routeBuy: "Купить",
    routeSell: "Продать",
    routeDeliverTo: "Везти в",

    // Table status
    foundDeals: "Найдено {count} сделок",
    scanPrompt: "Нажмите «Сканировать» для поиска сделок",
    scanStarting: "Запуск сканирования...",
    errorPrefix: "Ошибка: ",

    // Context menu
    copyItem: "Копировать предмет",
    copyBuyStation: "Копировать станцию покупки",
    copySellStation: "Копировать станцию продажи",

    // Table features
    filterPlaceholder: "Фильтр...",
    pinRow: "Закрепить",
    unpinRow: "Открепить",
    exportCSV: "Экспорт CSV",
    copyTable: "Копировать таблицу",
    clearFilters: "Сбросить фильтры",
    selected: "Выбрано: {count}",
    totalProfit: "Сумма прибыли",
    avgMargin: "Средняя маржа",
    showing: "Показано {shown} из {total}",
    pinned: "Закреплено: {count}",

    // Watchlist
    tabWatchlist: "Избранное",
    addToWatchlist: "В избранное",
    removeFromWatchlist: "Убрать из избранного",
    watchlistEmpty: "Избранное пусто",
    watchlistHint: "ПКМ на предмет → «В избранное»",
    watchlistThreshold: "Порог %",
    watchlistCurrentMargin: "Маржа %",
    watchlistCurrentProfit: "Прибыль",
    watchlistBuyAt: "Покупка",
    watchlistSellAt: "Продажа",
    watchlistAdded: "Добавлен",
    watchlistClickToEdit: "Клик для редактирования",
    watchlistTracked: "Отслеживается",
    watchlistAlerts: "Алертов",

    // Copy / Export
    copyRoute: "Копировать маршрут",
    copyTradeRoute: "Копировать маршрут (Buy → Sell)",
    copySystemAutopilot: "Копировать систему",
    copied: "Скопировано!",

    // Notifications
    alertTriggered: "Маржа {margin}% > порог {threshold}%",

    // History
    scanHistory: "История сканирований",
    historyEmpty: "Нет истории",
  },
  en: {
    // Header
    appTitle: "EVE Flipper",

    // Status
    sdeLoading: "SDE: loading...",
    sdeSystems: "systems",
    sdeTypes: "types",
    esiApi: "ESI API",
    esiUnavailable: "ESI API: unavailable",

    // Parameters
    system: "System",
    systemPlaceholder: "System...",
    cargoCapacity: "Cargo Capacity (m³)",
    buyRadius: "Buy Radius (jumps)",
    sellRadius: "Sell Radius (jumps)",
    minMargin: "Min Margin (%)",
    salesTax: "Sales Tax (%)",

    // Tabs
    tabRadius: "Flipper (radius)",
    tabRegion: "Regional Arbitrage",
    tabContracts: "Contract Arbitrage",

    // Buttons
    scan: "Scan",
    stop: "Stop",

    // Table
    colItem: "Item",
    colBuyPrice: "Buy ISK",
    colBuyStation: "Buy Station",
    colSellPrice: "Sell ISK",
    colSellStation: "Sell Station",
    colMargin: "Margin %",
    colUnitsToBuy: "Buy Qty",
    colAcceptQty: "Accept Qty",
    colProfit: "Profit ISK",
    colProfitPerJump: "ISK/Jump",
    colJumps: "Jumps",

    // Contract table
    colTitle: "Title",
    colContractPrice: "Contract Price",
    colMarketValue: "Market Value",
    colContractProfit: "Profit",
    colContractMargin: "Margin %",
    colVolume: "Volume m³",
    colStation: "Station",
    colItems: "Items",
    colContractJumps: "Jumps",
    colContractPPJ: "ISK/Jump",
    foundContracts: "Found {count} contracts",
    scanContractsPrompt: "Press \"Scan\" to search for contracts",

    // Route finder
    tabRoute: "Route",
    routeMinHops: "Min hops",
    routeMaxHops: "Max hops",
    routeFind: "Find routes",
    routeFound: "Found {count} routes",
    routePrompt: "Set parameters and press \"Find routes\"",
    routeColumn: "Route",
    routeHopsCol: "Hops",
    routeDetails: "Route details",
    routeTotalProfit: "Total profit",
    routeTotalJumps: "Jumps",
    routeJumpsUnit: "jumps",
    routeBuy: "Buy",
    routeSell: "Sell",
    routeDeliverTo: "Deliver to",

    // Table status
    foundDeals: "Found {count} deals",
    scanPrompt: "Press \"Scan\" to search for deals",
    scanStarting: "Starting scan...",
    errorPrefix: "Error: ",

    // Context menu
    copyItem: "Copy item name",
    copyBuyStation: "Copy buy station",
    copySellStation: "Copy sell station",

    // Table features
    filterPlaceholder: "Filter...",
    pinRow: "Pin",
    unpinRow: "Unpin",
    exportCSV: "Export CSV",
    copyTable: "Copy table",
    clearFilters: "Clear filters",
    selected: "Selected: {count}",
    totalProfit: "Total profit",
    avgMargin: "Avg margin",
    showing: "Showing {shown} of {total}",
    pinned: "Pinned: {count}",

    // Watchlist
    tabWatchlist: "Watchlist",
    addToWatchlist: "Add to watchlist",
    removeFromWatchlist: "Remove from watchlist",
    watchlistEmpty: "Watchlist is empty",
    watchlistHint: "Right-click item → \"Add to watchlist\"",
    watchlistThreshold: "Threshold %",
    watchlistCurrentMargin: "Margin %",
    watchlistCurrentProfit: "Profit",
    watchlistBuyAt: "Buy",
    watchlistSellAt: "Sell",
    watchlistAdded: "Added",
    watchlistClickToEdit: "Click to edit",
    watchlistTracked: "Tracked",
    watchlistAlerts: "Alerts",

    // Copy / Export
    copyRoute: "Copy route",
    copyTradeRoute: "Copy route (Buy → Sell)",
    copySystemAutopilot: "Copy system name",
    copied: "Copied!",

    // Notifications
    alertTriggered: "Margin {margin}% > threshold {threshold}%",

    // History
    scanHistory: "Scan history",
    historyEmpty: "No history",
  },
} as const;

export type TranslationKey = keyof (typeof translations)["ru"];

interface I18nContextType {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
}

const I18nContext = createContext<I18nContextType>(null!);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<Locale>(() => {
    const saved = localStorage.getItem("eve-flipper-locale");
    return (saved === "en" || saved === "ru") ? saved : "ru";
  });

  const changeLocale = useCallback((l: Locale) => {
    setLocale(l);
    localStorage.setItem("eve-flipper-locale", l);
  }, []);

  const t = useCallback(
    (key: TranslationKey, params?: Record<string, string | number>) => {
      let str: string = translations[locale][key] ?? key;
      if (params) {
        for (const [k, v] of Object.entries(params)) {
          str = str.replace(`{${k}}`, String(v));
        }
      }
      return str;
    },
    [locale]
  );

  return (
    <I18nContext.Provider value={{ locale, setLocale: changeLocale, t }}>
      {children}
    </I18nContext.Provider>
  );
}

export function useI18n() {
  return useContext(I18nContext);
}
