package assist

import (
	"strings"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/domain"
)

var trLower = strings.NewReplacer("İ", "i", "I", "ı")

func norm(s string) string { return strings.ToLower(trLower.Replace(s)) }

func has(text string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

// Rules is the offline Turkish knowledge base used when Gemini is not configured or fails.
func Rules(in Input) Diagnosis {
	text := " " + norm(in.Description) + " "
	d := Diagnosis{Provider: "rules"}
	portable := has(in.Product.Category, "phone", "laptop", "tablet", "headphones", "wearable", "camera")
	switch in.Problem {
	case domain.ProblemWontTurnOn:
		d.Summary = "Cihaz açılmıyor; çoğunlukla güç kaynağı, kablo veya pil kaynaklı basit bir nedeni olur."
		if portable {
			d.LikelyCauses = []Cause{
				{Cause: "Pil tamamen boşalmış", Likelihood: "high", Explanation: "Derin deşarj olan piller şarja takıldıktan sonra birkaç dakika tepki vermeyebilir."},
				{Cause: "Şarj kablosu veya adaptör arızalı", Likelihood: "medium", Explanation: "Kablo içinden kopmuş ya da adaptör yeterli güç vermiyor olabilir."},
				{Cause: "Yazılım donması", Likelihood: "medium", Explanation: "Cihaz açık ama ekran tepki vermiyor olabilir; zorla yeniden başlatma çözebilir."},
				{Cause: "Pil veya anakart arızası", Likelihood: "low", Explanation: "Yukarıdakiler çözmezse donanım arızası olasıdır; servis gerekir."},
			}
			d.Steps = []Step{
				{Title: "En az 30 dakika şarja takın", Detail: "Orijinal adaptör ve kabloyu, duvar prizine takılı şekilde kullanın."},
				{Title: "Farklı kablo ve adaptör deneyin", Detail: "Başka bir cihazda çalıştığını bildiğiniz bir şarj aleti kullanın."},
				{Title: "Zorla yeniden başlatın", Detail: "Güç tuşunu (bazı modellerde güç + ses kısma) 15–20 saniye basılı tutun."},
				{Title: "Şarj girişini kontrol edin", Detail: "Girişte toz veya kir varsa cihazı kapalıyken hafifçe üfleyin; metal nesne sokmayın."},
			}
		} else {
			d.LikelyCauses = []Cause{
				{Cause: "Priz veya uzatma kablosunda enerji yok", Likelihood: "high", Explanation: "Sigorta atmış, priz arızalı ya da grup prizin anahtarı kapalı olabilir."},
				{Cause: "Güç kablosu gevşek veya hasarlı", Likelihood: "medium", Explanation: "Kablo cihaz tarafında tam oturmamış olabilir."},
				{Cause: "Bekleme modu / kumanda sorunu", Likelihood: "medium", Explanation: "Kumanda pilleri bitmiş olabilir; cihaz üzerindeki tuşla deneyin."},
				{Cause: "Güç kartı arızası", Likelihood: "low", Explanation: "Elektrik dalgalanması sonrası sık görülür; servis gerekir."},
			}
			d.Steps = []Step{
				{Title: "Prizi başka bir cihazla test edin", Detail: "Aynı prize bir lamba veya şarj aleti takarak enerji olup olmadığını görün."},
				{Title: "Kabloyu çıkarıp yeniden takın", Detail: "Fişi çekin, 1 dakika bekleyin, iki ucu da sıkıca takın."},
				{Title: "Uzatma kablosu yerine doğrudan prize takın", Detail: "Grup prizler ve uzatmalar arıza kaynağı olabilir."},
				{Title: "Cihaz üzerindeki güç tuşunu deneyin", Detail: "Kumanda kullanıyorsanız pillerini değiştirin."},
			}
		}
	case domain.ProblemScreenLines:
		d.Summary = "Ekranda çizgi genellikle bağlantı, görüntü kaynağı veya panel arızasından kaynaklanır."
		d.LikelyCauses = []Cause{
			{Cause: "Görüntü kablosu veya bağlantısı", Likelihood: "high", Explanation: "HDMI/DisplayPort kablosu gevşek ya da hasarlı olabilir (harici ekranlarda)."},
			{Cause: "Yazılım veya ekran kartı sürücüsü", Likelihood: "medium", Explanation: "Çizgiler ekran görüntüsünde de görünüyorsa sorun paneldeyse değil, yazılımdadır."},
			{Cause: "Panel veya panel kablosu arızası", Likelihood: "medium", Explanation: "Çizgiler sabit kalıyor ve her kaynakta görünüyorsa panel arızası olasıdır."},
			{Cause: "Darbe veya baskı hasarı", Likelihood: "low", Explanation: "Ekrana baskı uygulanmışsa iç katmanlar zarar görmüş olabilir."},
		}
		d.Steps = []Step{
			{Title: "Cihazı yeniden başlatın", Detail: "Tamamen kapatıp 1 dakika sonra açın."},
			{Title: "Kabloları kontrol edin", Detail: "Harici ekranlarda kabloyu çıkarıp takın veya başka bir kablo deneyin."},
			{Title: "Başka bir görüntü kaynağı deneyin", Detail: "TV/monitörde farklı bir giriş, telefonda ekran görüntüsü alarak çizginin görünüp görünmediğine bakın."},
			{Title: "Yazılım güncellemelerini yükleyin", Detail: "İşletim sistemi ve ekran sürücüsünü güncelleyin."},
			{Title: "Fotoğraf çekin", Detail: "Servise başvururken arızayı gösteren bir fotoğraf işinizi kolaylaştırır."},
		}
		d.RecommendService = true
	case domain.ProblemPhysicalDamage:
		d.Summary = "Fiziksel hasar sonrası cihazı zorlamadan kullanmayı bırakıp hasarın boyutunu belgelemek en güvenli yoldur."
		d.LikelyCauses = []Cause{
			{Cause: "Darbe veya düşme sonucu iç parça hasarı", Likelihood: "high", Explanation: "Dışarıdan görünmeyen bağlantılar da zarar görmüş olabilir."},
			{Cause: "Kırık ekran veya kasa", Likelihood: "medium", Explanation: "Kırık cam keskin olabilir ve zamanla yayılır."},
			{Cause: "Sıvı teması", Likelihood: "low", Explanation: "Sıvı korozyona yol açar; belirtiler günler sonra çıkabilir."},
		}
		d.Steps = []Step{
			{Title: "Hasarın fotoğrafını çekin", Detail: "Farklı açılardan net fotoğraflar servis ve sigorta süreçleri için gereklidir."},
			{Title: "Cihazı kapatın ve şarja takmayın", Detail: "Özellikle kasa açıldıysa veya sıvı temas ettiyse kısa devre riski vardır."},
			{Title: "Kırık camı bantla kapatın", Detail: "Şeffaf bant kesilme riskini azaltır; cihazı kullanmaya devam etmeyin."},
			{Title: "Yetkili servise başvurun", Detail: "Onarım ücretini önceden yazılı olarak isteyin."},
		}
		d.RecommendService = true
	default:
		d.Summary = "Açıklamanıza göre bazı genel kontroller önerilir; sorun sürerse servise başvurun."
		d.LikelyCauses = []Cause{
			{Cause: "Geçici yazılım hatası", Likelihood: "medium", Explanation: "Yeniden başlatma birçok sorunu çözer."},
			{Cause: "Bağlantı veya aksesuar sorunu", Likelihood: "medium", Explanation: "Kablo, adaptör veya aksesuarlar arızalı olabilir."},
			{Cause: "Donanım arızası", Likelihood: "low", Explanation: "Basit kontroller çözmezse servis değerlendirmesi gerekir."},
		}
		d.Steps = []Step{
			{Title: "Cihazı yeniden başlatın", Detail: "Tamamen kapatıp birkaç dakika sonra açın."},
			{Title: "Kablo ve aksesuarları kontrol edin", Detail: "Mümkünse çalıştığını bildiğiniz başka bir kablo/aksesuar deneyin."},
			{Title: "Yazılımı güncelleyin", Detail: "Cihaz ve uygulama güncellemelerini yükleyin."},
			{Title: "Arızanın ne zaman oluştuğunu not edin", Detail: "Serviste sorunun tekrarlanabilmesi için koşulları yazın."},
		}
	}
	// Keyword extras regardless of the selected type.
	extra := func(c Cause) {
		if len(d.LikelyCauses) < 5 {
			d.LikelyCauses = append([]Cause{c}, d.LikelyCauses...)
		}
	}
	switch {
	case has(text, "şarj olmuyor", "sarj olmuyor", "şarj almıyor", "şarj tutmuyor", "batarya", "pil çabuk", "çabuk bitiyor"):
		extra(Cause{Cause: "Pil yıpranması veya şarj devresi", Likelihood: "high", Explanation: "Pil kapasitesi zamanla düşer; şarj girişi kirlenmiş de olabilir."})
	case has(text, "ısın", "isin", "sıcak", "aşırı ısı"):
		extra(Cause{Cause: "Aşırı ısınma", Likelihood: "high", Explanation: "Havalandırma delikleri tıkalı olabilir veya cihaz ağır yük altında çalışıyor olabilir."})
		d.Steps = append(d.Steps, Step{Title: "Havalandırmayı açık tutun", Detail: "Cihazı yumuşak yüzeylerde değil, düz ve sert bir yüzeyde kullanın."})
	case has(text, "ses yok", "ses gelmiyor", "hoparlör", "cızırt"):
		extra(Cause{Cause: "Ses ayarı veya hoparlör", Likelihood: "high", Explanation: "Sessiz mod, Bluetooth bağlantısı veya hoparlör arızası olabilir."})
	case has(text, "wifi", "wi-fi", "internet", "bluetooth", "bağlanmıyor"):
		extra(Cause{Cause: "Kablosuz bağlantı ayarları", Likelihood: "high", Explanation: "Ağ ayarlarını sıfırlamak veya cihazı eşleştirmeyi yenilemek çoğu zaman çözer."})
	case has(text, "su ", "sıvı", "islandı", "ıslandı", "döküldü"):
		extra(Cause{Cause: "Sıvı teması", Likelihood: "high", Explanation: "Sıvı kısa devreye ve korozyona yol açabilir."})
		d.RecommendService = true
	}
	if len(d.Steps) > 6 {
		d.Steps = d.Steps[:6]
	}
	return d
}

// safetyWarning returns an instruction to stop using the device when the description signals danger.
func safetyWarning(desc string) string {
	t := " " + norm(desc) + " "
	switch {
	case has(t, "duman", "yanık koku", "yanik koku", "yanık kokusu", "yanma kokusu", "alev", "kıvılcım", "kivilcim", "patla"):
		return "Duman, yanık kokusu veya kıvılcım ciddi bir tehlike işaretidir: cihazı hemen fişten çekin, kullanmayı bırakın ve yanıcı maddelerden uzak tutun."
	case has(t, "şişti", "şişmiş", "şişme", "sisti", "sismis"):
		return "Şişmiş pil yangın riski taşır: cihazı şarja takmayın, kullanmayı bırakın ve pile baskı uygulamayın."
	case has(t, "çarptı", "elektrik çarp", "carpti"):
		return "Elektrik çarpması riski var: cihazı fişten çekin ve servise başvurana kadar kullanmayın."
	case has(t, "su ", "su kaçtı", "sıvı", "islandı", "ıslandı", "döküldü"):
		return "Sıvı teması kısa devreye yol açabilir: cihazı kapatın, fişten çekin ve tamamen kuruyana kadar şarja takmayın veya açmayın."
	}
	return ""
}
