package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

// Структура для парсинга ответа OpenWeatherMap
type WeatherResponse struct {
	Name string `json:"name"`
	Main struct {
		Temp      float64 `json:"temp"`
		FeelsLike float64 `json:"feels_like"`
		Humidity  int     `json:"humidity"`
	} `json:"main"`
	Wind struct {
		Speed float64 `json:"speed"`
	} `json:"wind"`
	Weather []struct {
		Description string `json:"description"`
		Icon        string `json:"icon"`
	} `json:"weather"`
}

// Структура для парсинга прогноза на 5 дней
type ForecastResponse struct {
	List []struct {
		Dt   int64 `json:"dt"`
		Main struct {
			Temp      float64 `json:"temp"`
			FeelsLike float64 `json:"feels_like"`
			Humidity  int     `json:"humidity"`
		} `json:"main"`
		Weather []struct {
			Description string `json:"description"`
			Icon        string `json:"icon"`
		} `json:"weather"`
		Wind struct {
			Speed float64 `json:"speed"`
		} `json:"wind"`
		DtTxt string `json:"dt_txt"`
	} `json:"list"`
	City struct {
		Name string `json:"name"`
	} `json:"city"`
}

// Структура для кэширования погоды
type WeatherCache struct {
	data map[string]CacheItem
	mu   sync.RWMutex
}

type CacheItem struct {
	weatherData string
	timestamp   time.Time
}

// Создаем глобальный кэш
var weatherCache = &WeatherCache{
	data: make(map[string]CacheItem),
}

// Метод для получения данных из кэша
func (c *WeatherCache) Get(city string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.data[strings.ToLower(city)]
	if !exists {
		return "", false
	}

	// Проверяем актуальность кэша (30 минут)
	if time.Since(item.timestamp) > 30*time.Minute {
		return "", false
	}

	return item.weatherData, true
}

// Метод для сохранения данных в кэш
func (c *WeatherCache) Set(city, data string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[strings.ToLower(city)] = CacheItem{
		weatherData: data,
		timestamp:   time.Now(),
	}
}

func getWeather(city string) (string, error) {
	// Проверяем кэш
	if cachedData, ok := weatherCache.Get(city); ok {
		return cachedData, nil
	}

	apiKey := os.Getenv("OWM_API_KEY")

	url := fmt.Sprintf(
		"http://api.openweathermap.org/data/2.5/weather?q=%s&appid=%s&units=metric&lang=ru",
		city,
		apiKey,
	)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("ошибка запроса: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("город не найден или ошибка API")
	}

	var data WeatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("ошибка парсинга данных: %v", err)
	}

	weatherMsg := fmt.Sprintf(
		"🌤 Погода в %s:\n"+
			"🌡 Температура: %.0f°C (ощущается как %.0f°C)\n"+
			"💧 Влажность: %d%%\n"+
			"🌬 Ветер: %.0f м/с\n"+
			"📝 %s",
		data.Name,
		data.Main.Temp,
		data.Main.FeelsLike,
		data.Main.Humidity,
		data.Wind.Speed,
		data.Weather[0].Description,
	)

	// Сохраняем в кэш
	weatherCache.Set(city, weatherMsg)

	return weatherMsg, nil
}

// Функция для получения прогноза погоды на 5 дней
func getForecast(city string) (string, error) {
	apiKey := os.Getenv("OWM_API_KEY")

	url := fmt.Sprintf(
		"http://api.openweathermap.org/data/2.5/forecast?q=%s&appid=%s&units=metric&lang=ru",
		city,
		apiKey,
	)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("ошибка запроса: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("город не найден или ошибка API")
	}

	var data ForecastResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("ошибка парсинга данных: %v", err)
	}

	forecastMsg := fmt.Sprintf("🔮 Прогноз погоды на 5 дней для %s:\n\n", data.City.Name)

	// Группируем данные по дням
	currentDay := ""
	for i, item := range data.List {
		// Ограничиваем до 5 дней (максимум 15 элементов)
		if i >= 15 {
			break
		}

		// Из формата "2023-05-15 12:00:00" получаем только дату
		date := strings.Split(item.DtTxt, " ")[0]
		t, _ := time.Parse("2006-01-02", date)
		formattedDate := t.Format("02.01")

		// Если день изменился, выводим новый заголовок
		if currentDay != formattedDate {
			currentDay = formattedDate
			forecastMsg += fmt.Sprintf("\n📅 %s:\n", formattedDate)
		}

		// Время
		timeStr := strings.Split(item.DtTxt, " ")[1]
		timeStr = strings.Split(timeStr, ":")[0] + ":00"

		forecastMsg += fmt.Sprintf("⏰ %s: %.0f°C, %s\n",
			timeStr,
			item.Main.Temp,
			item.Weather[0].Description,
		)
	}

	return forecastMsg, nil
}

// Получение погоды по координатам
func getWeatherByCoords(lat, lon float64) (string, error) {
	apiKey := os.Getenv("OWM_API_KEY")

	url := fmt.Sprintf(
		"http://api.openweathermap.org/data/2.5/weather?lat=%.6f&lon=%.6f&appid=%s&units=metric&lang=ru",
		lat,
		lon,
		apiKey,
	)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("ошибка запроса: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ошибка получения данных API")
	}

	var data WeatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("ошибка парсинга данных: %v", err)
	}

	weatherMsg := fmt.Sprintf(
		"📍 Погода в вашем местоположении (%s):\n"+
			"🌡 Температура: %.0f°C (ощущается как %.0f°C)\n"+
			"💧 Влажность: %d%%\n"+
			"🌬 Ветер: %.0f м/с\n"+
			"📝 %s",
		data.Name,
		data.Main.Temp,
		data.Main.FeelsLike,
		data.Main.Humidity,
		data.Wind.Speed,
		data.Weather[0].Description,
	)

	return weatherMsg, nil
}

func main() {
	// Загружаем переменные окружения из .env файла
	if err := godotenv.Load(); err != nil {
		log.Printf("Ошибка загрузки .env файла: %v", err)
	}

	// Загружаем токены
	telegramToken := os.Getenv("TELEGRAM_TOKEN")
	if telegramToken == "" {
		log.Fatal("TELEGRAM_TOKEN не задан")
	}

	owmApiKey := os.Getenv("OWM_API_KEY")
	if owmApiKey == "" {
		log.Fatal("OWM_API_KEY не задан")
	}

	// Инициализируем бота
	bot, err := tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Fatalf("Ошибка инициализации бота: %v", err)
	}
	bot.Debug = true // Включить логирование (опционально)

	log.Printf("Бот запущен: @%s", bot.Self.UserName)

	// Настройка обновлений (updates)
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	// Карта последних запросов пользователей
	userLastCity := make(map[int64]string)

	// Обработка сообщений
	for update := range updates {
		// Обработка сообщений
		if update.Message != nil {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")

			// Обработка команд
			switch update.Message.Text {
			case "/start", "/help":
				msg.Text = "Привет! Я бот погоды. 🌤\n\n" +
					"Вы можете:\n" +
					"• Написать название города для получения текущей погоды\n" +
					"• Нажать кнопку 'Прогноз на 5 дней' для получения прогноза\n" +
					"• Отправить своё местоположение для погоды в вашей точке\n\n" +
					"Команды:\n" +
					"/start - Информация о боте\n" +
					"/help - Показать эту справку\n" +
					"/forecast - Прогноз на 5 дней для последнего запрошенного города"

				// Добавляем кнопку для отправки геолокации
				locationButton := tgbotapi.NewKeyboardButtonLocation("📍 Отправить местоположение")
				msg.ReplyMarkup = tgbotapi.NewReplyKeyboard(
					tgbotapi.NewKeyboardButtonRow(locationButton),
				)

			case "/forecast":
				// Проверяем, был ли у пользователя последний запрос города
				city, exists := userLastCity[update.Message.Chat.ID]
				if !exists {
					msg.Text = "Пожалуйста, сначала запросите погоду для какого-либо города."
				} else {
					forecast, err := getForecast(city)
					if err != nil {
						msg.Text = "❌ Ошибка: " + err.Error()
					} else {
						msg.Text = forecast
					}
				}

			default:
				city := update.Message.Text
				weatherInfo, err := getWeather(city)
				if err != nil {
					msg.Text = "❌ Ошибка: " + err.Error()
				} else {
					// Сохраняем последний запрошенный город
					userLastCity[update.Message.Chat.ID] = city

					msg.Text = weatherInfo

					// Добавляем кнопку для прогноза
					forecastButton := tgbotapi.NewInlineKeyboardButtonData("🔮 Прогноз на 5 дней", "forecast:"+city)
					msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
						tgbotapi.NewInlineKeyboardRow(forecastButton),
					)
				}
			}

			if _, err := bot.Send(msg); err != nil {
				log.Printf("Ошибка отправки сообщения: %v", err)
			}

			// Обработка местоположения
			if update.Message.Location != nil {
				weather, err := getWeatherByCoords(
					update.Message.Location.Latitude,
					update.Message.Location.Longitude,
				)

				replyMsg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
				if err != nil {
					replyMsg.Text = "❌ Ошибка получения погоды по координатам: " + err.Error()
				} else {
					replyMsg.Text = weather
				}

				if _, err := bot.Send(replyMsg); err != nil {
					log.Printf("Ошибка отправки сообщения с погодой по координатам: %v", err)
				}
			}
		}

		// Обработка колбэков (нажатия на кнопки)
		if update.CallbackQuery != nil {
			callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
			if _, err := bot.Request(callback); err != nil {
				log.Printf("Ошибка обработки колбэка: %v", err)
			}

			// Обработка колбэка для прогноза
			if strings.HasPrefix(update.CallbackQuery.Data, "forecast:") {
				city := strings.TrimPrefix(update.CallbackQuery.Data, "forecast:")

				forecast, err := getForecast(city)
				msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "")

				if err != nil {
					msg.Text = "❌ Ошибка: " + err.Error()
				} else {
					msg.Text = forecast
				}

				if _, err := bot.Send(msg); err != nil {
					log.Printf("Ошибка отправки сообщения с прогнозом: %v", err)
				}
			}
		}
	}
}
