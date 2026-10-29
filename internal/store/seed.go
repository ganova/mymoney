package store

import (
	"context"
	"strconv"
)

type seedAccount struct {
	name, typ, icon, color, aliases string
}

type seedCategory struct {
	name, kind, grp, icon, keywords string
}

// Data awal disusun mengikuti struktur spreadsheet budget bulanan yang biasa
// dipakai: tagihan bulanan, uang sehari-hari, belanja bulanan, cicilan, dll.
// Semua bisa diubah dari aplikasi.
var seedAccounts = []seedAccount{
	{"BCA", "bank", "🏦", "#0060af", "bca,debit,qris,m-banking,mbanking,mybca,klikbca,transfer,tf,atm"},
	{"Tunai", "cash", "💵", "#2e7d32", "cash,tunai,uang cash,dompet"},
	{"GoPay", "ewallet", "🟢", "#00a5cf", "gopay,gojek"},
	{"ShopeePay", "ewallet", "🟠", "#ee4d2d", "shopeepay,spay,shopee pay"},
	{"DANA", "ewallet", "🔵", "#118eea", "dana"},
	{"Flazz", "ewallet", "💳", "#5c6bc0", "flazz,e-money,emoney,kartu tol"},
}

var seedCategories = []seedCategory{
	// Tagihan bulanan
	{"Cicilan Rumah", "expense", "Tagihan Bulanan", "🏠", "bayar rumah,cicilan rumah,kpr,sewa rumah,kontrakan,kos"},
	{"Pengasuh Anak", "expense", "Tagihan Bulanan", "👶", "jaga anak,pengasuh,babysitter,jemput jaga,daycare,art,pembantu"},
	{"Sekolah Anak", "expense", "Tagihan Bulanan", "🎒", "tk,spp,sekolah,renang,les,buku sekolah,seragam"},
	{"Listrik & Pulsa", "expense", "Tagihan Bulanan", "⚡", "listrik,pln,token,pulsa,paket data,kuota"},
	{"Iuran Perumahan", "expense", "Tagihan Bulanan", "🏘️", "iuran,ipl,keamanan,sampah,rt,rw"},
	{"Internet", "expense", "Tagihan Bulanan", "🌐", "internet,wifi,indihome,rfnet,biznet,first media,myrepublic"},
	{"Ngaji", "expense", "Tagihan Bulanan", "📖", "ngaji,tpa,tpq,iqro,ustadz"},
	{"Laundry", "expense", "Tagihan Bulanan", "🧺", "laundry,laundy,cuci baju,setrika"},
	{"Transfer Istri", "expense", "Tagihan Bulanan", "💝", "tf istri,transfer istri,uang istri,istri,aderai"},
	{"Koperasi", "expense", "Tagihan Bulanan", "🤝", "koperasi,simpanan wajib"},
	// Sehari-hari
	{"Makan & Minum", "expense", "Sehari-hari", "🍜", "makan,makan siang,makan malam,sarapan,kopi,ngopi,gado,nasi,bebek,ayam,fried chicken,warung,warkop,gofood,grabfood,shopeefood,bakso,mie,jajan,snack,minum,boba,chagee,marugame,dapur"},
	{"Belanja Harian", "expense", "Sehari-hari", "🏪", "alfamart,indomaret,alfamidi,idm,midi,minimarket,lawson,family mart,circle k"},
	{"Belanja Sayur", "expense", "Sehari-hari", "🥬", "sayur,pasar,tukang sayur,ikan,daging,buah"},
	{"Bensin & Transport", "expense", "Sehari-hari", "⛽", "bensin,pertamax,pertalite,shell,spbu,parkir,tol,grab,gojek,ojol,gocar,grabcar,krl,mrt,transjakarta,busway"},
	{"Gas & Galon", "expense", "Sehari-hari", "🔥", "gas,galon,aqua,le minerale,elpiji,lpg"},
	// Belanja bulanan
	{"Belanja Bulanan", "expense", "Belanja Bulanan", "🛒", "belanja bulanan,aeon,superindo,hypermart,transmart,lotte,giant,hero,ranch market,grand lucky"},
	{"Belanja Online", "expense", "Belanja Bulanan", "📦", "shopee,tokopedia,tokped,lazada,blibli,tiktok shop"},
	// Cicilan & utang
	{"Cicilan & Paylater", "expense", "Cicilan & Utang", "💳", "kredivo,indodana,spaylater,shopee paylater,gopaylater,spinjam,credinex,akulaku,cicilan,kartu kredit,paylater"},
	// Lain-lain
	{"Zakat & Sedekah", "expense", "Lain-lain", "🕌", "zakat,sedekah,infaq,infak,donasi,wakaf,masjid"},
	{"Kesehatan", "expense", "Lain-lain", "💊", "obat,apotek,dokter,vitamin,klinik,rumah sakit,rs,bpjs,kimia farma,guardian"},
	{"Hiburan & Rekreasi", "expense", "Lain-lain", "🎡", "nonton,bioskop,xxi,cgv,waterland,rekreasi,liburan,game,netflix,spotify,youtube"},
	{"Pakaian & Perawatan", "expense", "Lain-lain", "👕", "baju,celana,sepatu,jahit,salon,potong rambut,cukur,barber,skincare,legging,sportstation,urban"},
	{"Olahraga", "expense", "Lain-lain", "🏃", "lari,run,gym,fitness,futsal,badminton,pocari run"},
	{"Sosial & Hadiah", "expense", "Lain-lain", "🎁", "kado,hadiah,kondangan,arisan,sumbangan,traktir"},
	{"Rumah & Perbaikan", "expense", "Lain-lain", "🔧", "servis,service,tukang,perabot,bengkel,perbaikan,ac"},
	{"Biaya Admin", "expense", "Lain-lain", "🧾", "admin,biaya admin,biaya transfer,materai"},
	{"Lain-lain", "expense", "Lain-lain", "📌", ""},
	// Pemasukan
	{"Gaji", "income", "Pemasukan", "💰", "gaji,gajian,payroll,salary"},
	{"Bonus & THR", "income", "Pemasukan", "🎉", "bonus,thr,insentif,tunjangan"},
	{"Refund & Cashback", "income", "Pemasukan", "↩️", "refund,cashback,pengembalian"},
	{"Pemasukan Lain", "income", "Pemasukan", "💵", "terima,transfer masuk,dapat,jual,hasil"},
}

func (s *Store) seed(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var firstAccount int64
	for i, a := range seedAccounts {
		res, err := tx.ExecContext(ctx, `INSERT INTO accounts (name, type, icon, color, aliases, sort) VALUES (?,?,?,?,?,?)`,
			a.name, a.typ, a.icon, a.color, a.aliases, i)
		if err != nil {
			return err
		}
		if i == 0 {
			firstAccount, _ = res.LastInsertId()
		}
	}
	for i, c := range seedCategories {
		if _, err := tx.ExecContext(ctx, `INSERT INTO categories (name, kind, grp, icon, keywords, sort, ledger_id) VALUES (?,?,?,?,?,?,1)`,
			c.name, c.kind, c.grp, c.icon, c.keywords, i); err != nil {
			return err
		}
	}
	defaults := map[string]string{
		"period_start_day":   "28",
		"default_account_id": strconv.FormatInt(firstAccount, 10),
		"ai_autosave":        "false",
		"owner_name":         "",
	}
	for k, v := range defaults {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}
